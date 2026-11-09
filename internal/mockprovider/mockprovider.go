// Package mockprovider is a deterministic, OpenAI-compatible chat completion
// server used for zero-cost demos and tests.
//
// Models fall into behaviour tiers so that replay experiments produce
// realistic, distinguishable outcomes:
//
//   - flagship (gpt-4o, gpt-4.1, claude-3-5-sonnet, opus, ...): full answer
//   - mini (*-mini): same answer with light rewording, faster
//   - haiku (*haiku*): shorter answer, fastest of the capable tiers
//   - small (*8b*, *nano*, *small*): terse answers, sometimes breaks JSON,
//     and returns occasional 503s
//
// Requests whose system prompt carries the Replay Lab judge marker are answered
// as pairwise judgements instead (see judgeAnswer).
//
// Requests with "stream": true are answered as OpenAI-style SSE (see
// writeStream). Two designated models misbehave mid-stream: a model whose name
// contains "stream-fail" sends an error event halfway through, and one
// containing "stream-stall" stops sending halfway and never ends the stream.
//
// Output depends only on (model, prompt), so repeated runs are reproducible.
package mockprovider

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// Options tunes the mock.
type Options struct {
	// LatencyScale multiplies simulated latency; 0 disables sleeping.
	LatencyScale float64
}

type tier int

const (
	tierFlagship tier = iota
	tierMini
	tierHaiku
	tierSmall
)

func tierOf(model string) tier {
	m := strings.ToLower(model)
	switch {
	case strings.Contains(m, "8b"), strings.Contains(m, "nano"), strings.Contains(m, "small"):
		return tierSmall
	case strings.Contains(m, "haiku"):
		return tierHaiku
	case strings.Contains(m, "mini"):
		return tierMini
	default:
		return tierFlagship
	}
}

// baseLatency returns the simulated latency range in milliseconds.
func (t tier) baseLatency() (min, spread int) {
	switch t {
	case tierMini:
		return 80, 40
	case tierHaiku:
		return 50, 30
	case tierSmall:
		return 40, 30
	default:
		return 150, 60
	}
}

type chatRequest struct {
	Model    string `json:"model"`
	Messages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
	ResponseFormat *struct {
		Type string `json:"type"`
	} `json:"response_format"`
	Stream        bool `json:"stream"`
	StreamOptions *struct {
		IncludeUsage bool `json:"include_usage"`
	} `json:"stream_options"`
}

func (r chatRequest) wantsUsageChunk() bool {
	return r.StreamOptions != nil && r.StreamOptions.IncludeUsage
}

// Handler returns the mock's HTTP handler (/v1/chat/completions and /health).
func Handler(opts Options) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"healthy","service":"bifrost-mockprovider"}`))
	})
	mux.HandleFunc("POST /v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		var req chatRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid payload: "+err.Error())
			return
		}
		if req.Model == "" || len(req.Messages) == 0 {
			writeErr(w, http.StatusBadRequest, "model and messages are required")
			return
		}

		var prompt, all strings.Builder
		for _, m := range req.Messages {
			all.WriteString(m.Content)
			if m.Role == "user" {
				prompt.Reset()
				prompt.WriteString(m.Content)
			}
		}
		t := tierOf(req.Model)
		h := hash(req.Model + "|" + prompt.String())

		if opts.LatencyScale > 0 {
			lo, spread := t.baseLatency()
			ms := float64(lo+int(h%uint64(spread))) * opts.LatencyScale
			select {
			case <-time.After(time.Duration(ms) * time.Millisecond):
			case <-r.Context().Done():
				return
			}
		}

		if isJudgeRequest(req.Messages) {
			content := judgeAnswer(req.Model, prompt.String())
			respond(w, r, req, h, content, all.Len())
			return
		}

		if t == tierSmall && h%100 < 8 {
			writeErr(w, http.StatusServiceUnavailable, "model overloaded, please retry")
			return
		}

		wantsJSON := (req.ResponseFormat != nil && req.ResponseFormat.Type == "json_object") ||
			strings.Contains(strings.ToLower(prompt.String()), "json")
		var content string
		if wantsJSON {
			content = jsonAnswer(prompt.String(), t, h)
		} else {
			content = textAnswer(prompt.String(), t)
		}

		respond(w, r, req, h, content, all.Len())
	})
	return mux
}

func respond(w http.ResponseWriter, r *http.Request, req chatRequest, h uint64, content string, inputLen int) {
	if req.Stream {
		writeStream(w, r, req, h, content, inputLen)
		return
	}
	writeCompletion(w, req.Model, h, content, inputLen)
}

// streamPieces splits content into deterministic deltas of up to three words,
// keeping every byte so the deltas concatenate back to content exactly.
func streamPieces(content string) []string {
	words := strings.SplitAfter(content, " ")
	var pieces []string
	for i := 0; i < len(words); i += 3 {
		pieces = append(pieces, strings.Join(words[i:min(i+3, len(words))], ""))
	}
	return pieces
}

// writeStream answers as OpenAI chat.completion.chunk SSE: a role chunk, the
// content deltas, a finish chunk, a usage chunk when stream_options asks for
// one, and [DONE].
func writeStream(w http.ResponseWriter, r *http.Request, req chatRequest, h uint64, content string, inputLen int) {
	flusher, _ := w.(http.Flusher)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	id := fmt.Sprintf("chatcmpl-mock-%x", h)
	created := time.Now().Unix()
	send := func(v any) {
		b, _ := json.Marshal(v)
		_, _ = fmt.Fprintf(w, "data: %s\n\n", b)
		if flusher != nil {
			flusher.Flush()
		}
	}
	chunk := func(delta map[string]string, finish any) map[string]any {
		return map[string]any{
			"id": id, "object": "chat.completion.chunk", "created": created, "model": req.Model,
			"choices": []map[string]any{{"index": 0, "delta": delta, "finish_reason": finish}},
		}
	}

	pieces := streamPieces(content)
	model := strings.ToLower(req.Model)
	failAt := -1
	if strings.Contains(model, "stream-fail") || strings.Contains(model, "stream-stall") {
		failAt = max(1, len(pieces)/2)
	}

	send(chunk(map[string]string{"role": "assistant", "content": ""}, nil))
	for i, piece := range pieces {
		if i == failAt {
			if strings.Contains(model, "stream-stall") {
				<-r.Context().Done() // never finish; wait for the client to give up
				return
			}
			send(map[string]any{"error": map[string]string{"message": "mock stream failure", "type": "mock_error"}})
			return
		}
		send(chunk(map[string]string{"content": piece}, nil))
	}
	send(chunk(map[string]string{}, "stop"))
	if req.wantsUsageChunk() {
		promptTokens := max(1, inputLen/4)
		completionTokens := max(1, len(content)/4)
		send(map[string]any{
			"id": id, "object": "chat.completion.chunk", "created": created, "model": req.Model,
			"choices": []any{},
			"usage": map[string]int{
				"prompt_tokens":     promptTokens,
				"completion_tokens": completionTokens,
				"total_tokens":      promptTokens + completionTokens,
			},
		})
	}
	_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	if flusher != nil {
		flusher.Flush()
	}
}

func writeCompletion(w http.ResponseWriter, model string, h uint64, content string, inputLen int) {
	promptTokens := max(1, inputLen/4)
	completionTokens := max(1, len(content)/4)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":      fmt.Sprintf("chatcmpl-mock-%x", h),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []map[string]any{{
			"index":         0,
			"message":       map[string]string{"role": "assistant", "content": content},
			"finish_reason": "stop",
		}},
		"usage": map[string]int{
			"prompt_tokens":     promptTokens,
			"completion_tokens": completionTokens,
			"total_tokens":      promptTokens + completionTokens,
		},
	})
}

// judgeMarker must match replaylab.JudgeSystemMarker. It is duplicated so the
// mock binary does not pull in the plugin and its SQLite driver.
const judgeMarker = "[replaylab-judge]"

func isJudgeRequest(msgs []struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}) bool {
	for _, m := range msgs {
		if m.Role == "system" && strings.HasPrefix(m.Content, judgeMarker) {
			return true
		}
	}
	return false
}

// judgeAnswer plays a pairwise judge. A model whose name contains "biased"
// always picks position A. Any other judge prefers the more complete response
// (by word count) and calls a tie when the two are within 20% of each other.
func judgeAnswer(model, prompt string) string {
	reply := func(winner, reason string) string {
		b, _ := json.Marshal(map[string]string{"winner": winner, "reason": reason})
		return string(b)
	}
	if strings.Contains(strings.ToLower(model), "biased") {
		return reply("A", "The first response is better.")
	}
	a := len(strings.Fields(fencedBlock(prompt, "RESPONSE_A")))
	b := len(strings.Fields(fencedBlock(prompt, "RESPONSE_B")))
	longer := max(a, b)
	switch {
	case longer == 0 || float64(abs(a-b))/float64(longer) <= 0.2:
		return reply("tie", "Both responses answer the prompt equally well.")
	case a > b:
		return reply("A", "Response A is more complete.")
	default:
		return reply("B", "Response B is more complete.")
	}
}

// fencedBlock extracts the body of a "<<<NAME id>>> ... <<<END_NAME id>>>" block.
func fencedBlock(text, name string) string {
	open := "<<<" + name + " "
	start := strings.Index(text, open)
	if start < 0 {
		return ""
	}
	rest := text[start+len(open):]
	idEnd := strings.Index(rest, ">>>\n")
	if idEnd < 0 {
		return ""
	}
	id, body := rest[:idEnd], rest[idEnd+len(">>>\n"):]
	end := strings.Index(body, "\n<<<END_"+name+" "+id+">>>")
	if end < 0 {
		return ""
	}
	return body[:end]
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

var (
	quotedCSVHeader = regexp.MustCompile(`'([a-z_]+(?:,[a-z_]+)+)'`)
	containingList  = regexp.MustCompile(`(?i)containing ([a-z_]+(?:, [a-z_]+)*,? and [a-z_]+)`)
	wordRe          = regexp.MustCompile(`[A-Za-z][A-Za-z0-9-]{3,}`)
)

var stopwords = map[string]bool{
	"this": true, "that": true, "with": true, "from": true, "into": true, "your": true,
	"what": true, "which": true, "about": true, "their": true, "versus": true, "user": true,
	"query": true, "explain": true, "write": true, "generate": true, "summarize": true,
}

// keysFor infers the JSON keys a prompt asks for.
func keysFor(prompt string) []string {
	if m := quotedCSVHeader.FindStringSubmatch(prompt); m != nil {
		return strings.Split(m[1], ",")
	}
	if m := containingList.FindStringSubmatch(prompt); m != nil {
		list := strings.NewReplacer(", and ", ",", " and ", ",", ", ", ",").Replace(m[1])
		return strings.Split(strings.ToLower(list), ",")
	}
	return []string{"result", "confidence"}
}

func jsonAnswer(prompt string, t tier, h uint64) string {
	keys := keysFor(prompt)
	obj := make(map[string]string, len(keys))
	for _, k := range keys {
		obj[k] = "extracted " + k
	}
	switch t {
	case tierHaiku:
		if len(keys) > 2 && h%4 == 0 {
			delete(obj, keys[len(keys)-1]) // occasionally drops a field
		}
	case tierSmall:
		if h%10 < 4 {
			return "Sure! Here is the data you asked for: " + strings.Join(keys, ", ") // broken contract
		}
	}
	b, _ := json.Marshal(obj)
	return string(b)
}

func textAnswer(prompt string, t tier) string {
	var words []string
	for _, w := range wordRe.FindAllString(prompt, -1) {
		if !stopwords[strings.ToLower(w)] {
			words = append(words, strings.ToLower(w))
		}
	}
	topic := strings.Join(firstN(words, 6), " ")
	points := strings.Join(firstN(skip(words, 6), 6), ", ")
	if points == "" {
		points = topic
	}

	s1 := fmt.Sprintf("Here is a direct answer about %s.", topic)
	s2 := fmt.Sprintf("The key points are %s.", points)
	s3 := "In production, validate the approach with tests, monitoring and a staged rollout."

	switch t {
	case tierMini:
		return s1 + " " + strings.Replace(s2, "key points", "main points", 1) + " " + s3
	case tierHaiku:
		return s1 + " " + s2
	case tierSmall:
		return "Short answer: " + topic + "."
	default:
		return s1 + " " + s2 + " " + s3
	}
}

func firstN(xs []string, n int) []string {
	if len(xs) < n {
		return xs
	}
	return xs[:n]
}

func skip(xs []string, n int) []string {
	if len(xs) < n {
		return nil
	}
	return xs[n:]
}

func hash(s string) uint64 {
	f := fnv.New64a()
	_, _ = f.Write([]byte(s))
	return f.Sum64()
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"message": msg, "type": "mock_error"}})
}
