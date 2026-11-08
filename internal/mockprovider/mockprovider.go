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

		promptTokens := max(1, len(all.String())/4)
		completionTokens := max(1, len(content)/4)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      fmt.Sprintf("chatcmpl-mock-%x", h),
			"object":  "chat.completion",
			"created": time.Now().Unix(),
			"model":   req.Model,
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
	})
	return mux
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
