package replaylab

import (
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// A streamed capture is built up while the stream is in flight and finalised
// exactly once, into an ordinary CaptureRecord that goes through the same
// scrub and write path as every other capture. Both capture paths (the
// Bifrost hooks and the sidecar proxy) drive it through RecordStreamRequest,
// AppendStreamContent and FinishStream.

// streamCapture is the in-flight state of one streamed capture.
type streamCapture struct {
	mu           sync.Mutex
	done         bool
	content      strings.Builder
	limit        int  // bytes of content kept at most
	seenBytes    int  // bytes of content received, kept or not
	truncated    bool // content past limit was discarded
	firstTokenAt time.Time
}

// StreamOutcome is how a stream ended.
type StreamOutcome struct {
	// State is CompletionStateComplete, CompletionStateError or
	// CompletionStateAbandoned. A complete stream whose content overflowed the
	// cap is stored as CompletionStateTruncated.
	State string
	// StatusCode defaults to 200 for complete streams, 502 for errors and 499
	// for abandoned streams.
	StatusCode   int
	ErrorMessage string
	// HasUsage is true when the provider reported token usage. Otherwise
	// tokens are estimated from content length and UsageEstimated is set.
	HasUsage         bool
	PromptTokens     int
	CompletionTokens int
	// Truncated reports that the capture path had to drop part of the stream
	// before it reached the content buffer (for example, an oversize SSE
	// line). A complete stream with Truncated set is stored as truncated.
	Truncated bool
}

// statusClientClosedRequest is the de facto status for a client that hung up.
const statusClientClosedRequest = 499

// RecordStreamRequest starts a streamed capture. It applies the same
// sampling and opt-in/opt-out rules as RecordRequest and reports whether the
// stream is being captured.
func (p *Plugin) RecordStreamRequest(reqID, virtualKey, tenant, provider, model, messagesJSON, paramsJSON, tag string) bool {
	if reqID == "" || !p.queue.ShouldCapture(virtualKey) {
		return false
	}
	p.pendingMap.Store(reqID, &PendingCaptureContext{
		StartTime:    time.Now(),
		RequestID:    reqID,
		VirtualKey:   virtualKey,
		Tenant:       tenant,
		Provider:     provider,
		Model:        model,
		MessagesJSON: messagesJSON,
		ParamsJSON:   paramsJSON,
		Tag:          tag,
		stream:       &streamCapture{limit: p.queue.Config().StreamCaptureLimit()},
	})
	return true
}

// AppendStreamContent adds one chunk's assistant text to an in-flight
// streamed capture. Content past the per-stream cap is discarded. It is a
// no-op once the capture has been finalised.
func (p *Plugin) AppendStreamContent(reqID, delta string) {
	if delta == "" {
		return
	}
	val, ok := p.pendingMap.Load(reqID)
	if !ok {
		return
	}
	sc := val.(*PendingCaptureContext).stream
	if sc == nil {
		return
	}
	sc.mu.Lock()
	defer sc.mu.Unlock()
	if sc.done {
		return
	}
	if sc.firstTokenAt.IsZero() {
		sc.firstTokenAt = time.Now()
	}
	sc.seenBytes += len(delta)
	if sc.truncated {
		return
	}
	room := sc.limit - sc.content.Len()
	if len(delta) > room {
		// Keep what fits, cut on a rune boundary so stored text stays valid UTF-8.
		cut := room
		for cut > 0 && !utf8.RuneStart(delta[cut]) {
			cut--
		}
		sc.content.WriteString(delta[:cut])
		sc.truncated = true
		return
	}
	sc.content.WriteString(delta)
}

// StreamReceivedContent reports whether an in-flight streamed capture has
// received any content yet.
func (p *Plugin) StreamReceivedContent(reqID string) bool {
	val, ok := p.pendingMap.Load(reqID)
	if !ok {
		return false
	}
	sc := val.(*PendingCaptureContext).stream
	if sc == nil {
		return false
	}
	sc.mu.Lock()
	defer sc.mu.Unlock()
	return sc.seenBytes > 0
}

// FinishStream finalises a streamed capture and enqueues it. Only the first
// call for a capture has any effect, so racing end signals (final chunk,
// context end, janitor) are safe.
func (p *Plugin) FinishStream(reqID string, out StreamOutcome) {
	val, ok := p.pendingMap.LoadAndDelete(reqID)
	if !ok {
		return
	}
	pending := val.(*PendingCaptureContext)
	sc := pending.stream
	if sc == nil {
		return // not a stream; leave it to RecordResponse's caller
	}
	sc.mu.Lock()
	sc.done = true
	content := sc.content.String()
	seen := sc.seenBytes
	truncated := sc.truncated || out.Truncated
	firstTokenAt := sc.firstTokenAt
	sc.mu.Unlock()

	now := time.Now()
	state := out.State
	if state == "" {
		state = CompletionStateComplete
	}
	if state == CompletionStateComplete && truncated {
		state = CompletionStateTruncated
	}
	status := out.StatusCode
	if status == 0 {
		switch state {
		case CompletionStateError:
			status = http.StatusBadGateway
		case CompletionStateAbandoned:
			status = statusClientClosedRequest
		default:
			status = http.StatusOK
		}
	}

	prompt, completion := out.PromptTokens, out.CompletionTokens
	estimated := false
	if !out.HasUsage || (prompt == 0 && completion == 0) {
		prompt, completion = estimateTokens(len(pending.MessagesJSON)), estimateTokens(seen)
		estimated = true
	}

	rec := &CaptureRecord{
		RequestID:        pending.RequestID,
		Timestamp:        pending.StartTime,
		VirtualKey:       pending.VirtualKey,
		Tenant:           pending.Tenant,
		Provider:         pending.Provider,
		Model:            pending.Model,
		MessagesJSON:     pending.MessagesJSON,
		ParamsJSON:       pending.ParamsJSON,
		ResponseJSON:     content,
		PromptTokens:     prompt,
		CompletionTokens: completion,
		TotalTokens:      prompt + completion,
		LatencyMs:        now.Sub(pending.StartTime).Milliseconds(),
		CostEstimate:     EstimateCost(pending.Model, prompt, completion),
		StatusCode:       status,
		ErrorMessage:     out.ErrorMessage,
		IsStreaming:      true,
		Tag:              pending.Tag,
		CompletionState:  state,
		UsageEstimated:   estimated,
	}
	if !firstTokenAt.IsZero() {
		ttft := firstTokenAt.Sub(pending.StartTime).Milliseconds()
		rec.TimeToFirstTokenMs = &ttft
	}
	if state == CompletionStateTruncated {
		p.queue.streamsTruncated.Add(1)
	}
	if p.queue.Enqueue(rec) {
		p.queue.streamedTotal.Add(1)
	}
}

// estimateTokens approximates a token count from a byte length (~4 bytes per
// token for English text), never returning zero for non-empty input.
func estimateTokens(n int) int {
	if n <= 0 {
		return 0
	}
	return max(1, n/4)
}
