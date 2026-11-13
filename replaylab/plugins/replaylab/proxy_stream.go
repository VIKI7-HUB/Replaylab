package replaylab

import (
	"bytes"
	"encoding/json"
	"net/http"
	"sync"
)

// The sidecar proxy captures a streamed response without touching the path
// that writes to the client: the relay loop hands each chunk to an sseCapture
// after writing and flushing it, and a separate goroutine parses the SSE
// "data:" lines into the shared in-flight stream model (see stream.go).

const (
	// maxStreamParseBacklog bounds relayed bytes waiting to be parsed. The
	// parser normally keeps up; if it ever falls this far behind, the rest of
	// the stream is not captured and the capture is stored as truncated.
	maxStreamParseBacklog = 8 << 20
	// maxSSELineBytes bounds one SSE line. A longer line is skipped and the
	// capture is stored as truncated.
	maxSSELineBytes = 1 << 20
)

// sseCapture parses one relayed SSE stream on the side.
type sseCapture struct {
	p     *Plugin
	reqID string
	wake  chan struct{} // capacity 1: "there is something to look at"

	mu         sync.Mutex
	backlog    []byte // relayed, not yet parsed
	overflow   bool   // backlog exceeded maxStreamParseBacklog
	ended      bool   // the relay finished; no more feed calls take effect
	clientGone bool
	readErr    error

	// Parser state, owned by the run goroutine.
	work        []byte
	line        []byte
	skipLine    bool // dropping the rest of an oversize line
	lineDropped bool
	sawDone     bool
	streamErr   string
	hasUsage    bool
	prompt      int
	completion  int
}

func newSSECapture(p *Plugin, reqID string) *sseCapture {
	return &sseCapture{p: p, reqID: reqID, wake: make(chan struct{}, 1)}
}

// feed queues bytes the client has already received. It never blocks on the
// parser.
func (s *sseCapture) feed(b []byte) {
	s.mu.Lock()
	switch {
	case s.ended || s.overflow:
	case len(s.backlog)+len(b) > maxStreamParseBacklog:
		s.overflow = true
		s.backlog = nil
	default:
		s.backlog = append(s.backlog, b...)
	}
	s.mu.Unlock()
	s.signal()
}

// finish records how the relay ended. The run goroutine finalises the capture
// once it has parsed everything fed before this call.
func (s *sseCapture) finish(clientGone bool, readErr error) {
	s.mu.Lock()
	if !s.ended {
		s.ended, s.clientGone, s.readErr = true, clientGone, readErr
	}
	s.mu.Unlock()
	s.signal()
}

func (s *sseCapture) signal() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// run parses fed bytes until the relay has ended and the backlog is drained,
// then finalises the capture exactly once.
func (s *sseCapture) run() {
	for range s.wake {
		s.mu.Lock()
		s.work = append(s.work[:0], s.backlog...)
		s.backlog = s.backlog[:0]
		ended := s.ended
		s.mu.Unlock()

		s.parse(s.work)
		if ended {
			s.finalise()
			return
		}
	}
}

func (s *sseCapture) parse(data []byte) {
	for len(data) > 0 {
		i := bytes.IndexByte(data, '\n')
		chunk := data
		if i >= 0 {
			chunk = data[:i]
		}
		if !s.skipLine {
			if len(s.line)+len(chunk) > maxSSELineBytes {
				s.line, s.skipLine, s.lineDropped = s.line[:0], true, true
			} else {
				s.line = append(s.line, chunk...)
			}
		}
		if i < 0 {
			return
		}
		if !s.skipLine {
			s.processLine(s.line)
		}
		s.line, s.skipLine = s.line[:0], false
		data = data[i+1:]
	}
}

// sseChunk is the part of an OpenAI chat.completion.chunk event the capture
// needs.
type sseChunk struct {
	Choices []struct {
		Index int `json:"index"`
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (s *sseCapture) processLine(line []byte) {
	if s.sawDone || s.streamErr != "" {
		return
	}
	payload, ok := bytes.CutPrefix(bytes.TrimRight(line, "\r"), []byte("data:"))
	if !ok {
		return // comments, event names, ids and blank separators
	}
	payload = bytes.TrimSpace(payload)
	if string(payload) == "[DONE]" {
		s.sawDone = true
		return
	}
	var c sseChunk
	if err := json.Unmarshal(payload, &c); err != nil {
		return
	}
	if c.Error != nil {
		s.streamErr = c.Error.Message
		if s.streamErr == "" {
			s.streamErr = "stream error"
		}
		return
	}
	for _, ch := range c.Choices {
		if ch.Index == 0 {
			s.p.AppendStreamContent(s.reqID, ch.Delta.Content)
		}
	}
	if c.Usage != nil {
		s.hasUsage, s.prompt, s.completion = true, c.Usage.PromptTokens, c.Usage.CompletionTokens
	}
}

func (s *sseCapture) finalise() {
	if len(s.line) > 0 && !s.skipLine {
		s.processLine(s.line) // a last line without a trailing newline
	}
	s.mu.Lock()
	clientGone, readErr, overflow := s.clientGone, s.readErr, s.overflow
	s.mu.Unlock()

	out := StreamOutcome{
		State:            CompletionStateComplete,
		HasUsage:         s.hasUsage,
		PromptTokens:     s.prompt,
		CompletionTokens: s.completion,
		Truncated:        overflow || s.lineDropped,
	}
	failed := func(msg string) {
		out.StatusCode, out.ErrorMessage = http.StatusBadGateway, msg
		// An error before any content is an ordinary failed request; one after
		// content arrived leaves a partial answer.
		if s.p.StreamReceivedContent(s.reqID) {
			out.State = CompletionStateError
		}
	}
	switch {
	case s.streamErr != "":
		failed(s.streamErr)
	case s.sawDone:
		// Complete, even if the client hung up right after the last event.
	case clientGone:
		out.State, out.ErrorMessage = CompletionStateAbandoned, "client disconnected before the stream finished"
	case readErr != nil:
		failed("upstream stream failed: " + readErr.Error())
	}
	s.p.FinishStream(s.reqID, out)
}
