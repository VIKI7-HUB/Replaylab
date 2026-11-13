package tests

// Streamed capture through the sidecar proxy: streamed chat requests go
// through ProxyHandler to an SSE upstream (the mock provider, or a scripted
// server when the exact bytes matter).

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/maximhq/bifrost/plugins/replaylab/internal/mockprovider"
	"github.com/maximhq/bifrost/plugins/replaylab/plugins/replaylab"
)

// newProxySetup starts the mock provider and a Replay Lab proxy in front of it.
func newProxySetup(t *testing.T) (*replaylab.Plugin, string) {
	t.Helper()
	mock := httptest.NewServer(mockprovider.Handler(mockprovider.Options{}))
	t.Cleanup(mock.Close)
	return newLabProxy(t, mock.URL)
}

func newLabProxy(t *testing.T, upstream string) (*replaylab.Plugin, string) {
	t.Helper()
	lab := newLab(t, upstream)
	proxy := httptest.NewServer(lab.ProxyHandler())
	t.Cleanup(proxy.Close)
	return lab, proxy.URL
}

func chatBody(model string, stream, includeUsage bool) string {
	body := map[string]any{
		"model":    model,
		"messages": []map[string]string{{"role": "user", "content": streamPrompt}},
	}
	if stream {
		body["stream"] = true
	}
	if includeUsage {
		body["stream_options"] = map[string]bool{"include_usage": true}
	}
	b, _ := json.Marshal(body)
	return string(b)
}

func postProxy(t *testing.T, proxyURL, body string) (*http.Response, []byte) {
	t.Helper()
	resp, err := http.Post(proxyURL+"/v1/chat/completions", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp, b
}

// sseText concatenates the content deltas a client sees in an SSE body.
func sseText(t *testing.T, body []byte) string {
	t.Helper()
	var sb strings.Builder
	sc := bufio.NewScanner(bytes.NewReader(body))
	for sc.Scan() {
		data, ok := strings.CutPrefix(sc.Text(), "data: ")
		if !ok || data == "[DONE]" {
			continue
		}
		var c struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if json.Unmarshal([]byte(data), &c) == nil && len(c.Choices) > 0 {
			sb.WriteString(c.Choices[0].Delta.Content)
		}
	}
	return sb.String()
}

func streamedCapture(t *testing.T, s replaylab.Storage, model string) *replaylab.CaptureRecord {
	t.Helper()
	streamed := true
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		caps, _ := s.GetCaptures(replaylab.CaptureFilter{Model: model, Streamed: &streamed, Limit: 10})
		if len(caps) > 0 {
			return caps[0]
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("no streamed capture stored for %s", model)
	return nil
}

// scriptedSSE is an upstream that writes fixed SSE pieces, flushing after
// each one, and records the request body it received.
type scriptedSSE struct {
	pieces []string
	mu     sync.Mutex
	got    []byte
}

func (s *scriptedSSE) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b, _ := io.ReadAll(r.Body)
	s.mu.Lock()
	s.got = b
	s.mu.Unlock()
	w.Header().Set("Content-Type", "text/event-stream")
	for _, p := range s.pieces {
		_, _ = io.WriteString(w, p)
		w.(http.Flusher).Flush()
		time.Sleep(2 * time.Millisecond)
	}
}

func (s *scriptedSSE) all() string { return strings.Join(s.pieces, "") }

func TestProxyStreamRelaysExactBytesAndReassembles(t *testing.T) {
	// Pieces split events mid-line and mid-JSON, include a comment, CRLF line
	// ends and a multi-byte rune split across writes.
	up := &scriptedSSE{pieces: []string{
		": keep-alive\n\n",
		`data: {"choices":[{"index":0,"delta":{"role":"assistant","content":""}}]}` + "\n\n",
		`data: {"choices":[{"index":0,"delta":{"content":"Hello"}}]}` + "\n\n" + `data: {"choi`,
		`ces":[{"index":0,"delta":{"content":", wörld"}}]}` + "\r\n\r\n",
		"data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\" caf\xc3",
		"\xa9!\"}}]}\n\n",
		`data: {"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}` + "\n\n",
		`data: {"choices":[],"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18}}` + "\n\n",
		"data: [DONE]\n\n",
	}}
	upstream := httptest.NewServer(up)
	defer upstream.Close()
	lab, proxyURL := newLabProxy(t, upstream.URL)

	sent := chatBody("gpt-4o", true, true)
	resp, got := postProxy(t, proxyURL, sent)
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if string(got) != up.all() {
		t.Fatalf("client bytes differ from upstream bytes:\n got %q\nwant %q", got, up.all())
	}
	up.mu.Lock()
	forwarded := string(up.got)
	up.mu.Unlock()
	if forwarded != sent {
		t.Errorf("request was modified:\n got %s\nwant %s", forwarded, sent)
	}

	c := streamedCapture(t, lab.Storage(), "gpt-4o")
	if want := "Hello, wörld café!"; c.ResponseJSON != want {
		t.Errorf("response_json = %q, want %q", c.ResponseJSON, want)
	}
	if c.CompletionState != replaylab.CompletionStateComplete || c.StatusCode != 200 {
		t.Errorf("state=%s status=%d, want complete/200", c.CompletionState, c.StatusCode)
	}
	if c.PromptTokens != 11 || c.CompletionTokens != 7 || c.UsageEstimated {
		t.Errorf("usage = %d/%d estimated=%v, want 11/7 from the usage chunk", c.PromptTokens, c.CompletionTokens, c.UsageEstimated)
	}
	if c.TimeToFirstTokenMs == nil || *c.TimeToFirstTokenMs > c.LatencyMs {
		t.Errorf("time_to_first_token_ms = %v, latency %d", c.TimeToFirstTokenMs, c.LatencyMs)
	}
	if strings.Contains(c.ParamsJSON, "stream") {
		t.Errorf("stream params stored: %s", c.ParamsJSON)
	}
}

func TestProxyStreamMatchesNonStreamedAnswerAndEstimatesUsage(t *testing.T) {
	lab, proxyURL := newProxySetup(t)

	_, plain := postProxy(t, proxyURL, chatBody("gpt-4o-mini", false, false))
	var completion struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(plain, &completion); err != nil || len(completion.Choices) == 0 {
		t.Fatalf("non-streamed reply: %v %s", err, plain)
	}
	want := completion.Choices[0].Message.Content

	// No stream_options: the stream carries no usage and the proxy must not
	// ask for it.
	resp, body := postProxy(t, proxyURL, chatBody("gpt-4o-mini", true, false))
	if resp.StatusCode != 200 || sseText(t, body) != want {
		t.Fatalf("client saw status %d text %q, want %q", resp.StatusCode, sseText(t, body), want)
	}
	c := streamedCapture(t, lab.Storage(), "gpt-4o-mini")
	if c.ResponseJSON != want {
		t.Errorf("response_json = %q, want %q", c.ResponseJSON, want)
	}
	if !c.UsageEstimated || c.PromptTokens == 0 || c.CompletionTokens == 0 {
		t.Errorf("usage = %d/%d estimated=%v, want estimated non-zero tokens", c.PromptTokens, c.CompletionTokens, c.UsageEstimated)
	}
	if c.CompletionState != replaylab.CompletionStateComplete {
		t.Errorf("completion_state = %s", c.CompletionState)
	}
}

func TestProxyStreamMidStreamErrorIsStoredAsError(t *testing.T) {
	lab, proxyURL := newProxySetup(t)
	model := "gpt-4o-stream-fail"

	_, body := postProxy(t, proxyURL, chatBody(model, true, true))
	partial := sseText(t, body)
	if partial == "" || !strings.Contains(string(body), "mock stream failure") {
		t.Fatalf("client should see partial content then the error event: %q", body)
	}
	c := streamedCapture(t, lab.Storage(), model)
	if c.CompletionState != replaylab.CompletionStateError || c.StatusCode < 400 {
		t.Errorf("state=%s status=%d, want error with a failure status", c.CompletionState, c.StatusCode)
	}
	if c.ResponseJSON != partial || !strings.Contains(c.ErrorMessage, "mock stream failure") {
		t.Errorf("stored %q / %q, want partial %q and the error message", c.ResponseJSON, c.ErrorMessage, partial)
	}
	assertNotReplayable(t, lab.Storage(), model)
}

func TestProxyStreamAbandonedByClient(t *testing.T) {
	lab, proxyURL := newProxySetup(t)
	model := "gpt-4o-stream-stall"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, proxyURL+"/v1/chat/completions",
		strings.NewReader(chatBody(model, true, true)))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	// Read until some content has arrived, then hang up.
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		if strings.Contains(sc.Text(), `"content":"`) && !strings.Contains(sc.Text(), `"content":""`) {
			break
		}
	}
	cancel()
	resp.Body.Close()

	c := streamedCapture(t, lab.Storage(), model)
	if c.CompletionState != replaylab.CompletionStateAbandoned {
		t.Errorf("completion_state = %s, want abandoned", c.CompletionState)
	}
	if c.ResponseJSON == "" {
		t.Error("content received before the hang-up should be kept")
	}
	assertNotReplayable(t, lab.Storage(), model)
}

func TestProxyStreamTruncatedAtCaptureCap(t *testing.T) {
	lab, proxyURL := newProxySetup(t)
	setConfig(t, lab, `{"max_stream_capture_bytes":24}`)

	_, body := postProxy(t, proxyURL, chatBody("gpt-4o", true, true))
	full := sseText(t, body)
	if len(full) <= 24 {
		t.Fatalf("answer too short to test truncation: %q", full)
	}
	c := streamedCapture(t, lab.Storage(), "gpt-4o")
	if c.CompletionState != replaylab.CompletionStateTruncated {
		t.Errorf("completion_state = %s, want truncated", c.CompletionState)
	}
	if len(c.ResponseJSON) > 24 || !strings.HasPrefix(full, c.ResponseJSON) {
		t.Errorf("stored %q, want a prefix of at most 24 bytes of %q", c.ResponseJSON, full)
	}
	if c.StatusCode != 200 || c.UsageEstimated {
		t.Errorf("status=%d estimated=%v; usage should still come from the usage chunk", c.StatusCode, c.UsageEstimated)
	}
	assertNotReplayable(t, lab.Storage(), "gpt-4o")
	if m := metricsText(t, lab); !strings.Contains(m, "bifrost_replaylab_stream_captures_truncated_total 1") {
		t.Errorf("truncation metric missing:\n%s", m)
	}
}

func TestProxyStreamUpstreamRejectionIsAFailedStreamedCapture(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"message":"bad key"}}`, http.StatusUnauthorized)
	}))
	defer upstream.Close()
	lab, proxyURL := newLabProxy(t, upstream.URL)

	resp, body := postProxy(t, proxyURL, chatBody("gpt-4o", true, false))
	if resp.StatusCode != http.StatusUnauthorized || !strings.Contains(string(body), "bad key") {
		t.Fatalf("client got %d %s, want the upstream's 401", resp.StatusCode, body)
	}
	c := streamedCapture(t, lab.Storage(), "gpt-4o")
	if c.StatusCode != http.StatusUnauthorized || !strings.Contains(c.ErrorMessage, "bad key") {
		t.Errorf("status=%d error=%q", c.StatusCode, c.ErrorMessage)
	}
	assertNotReplayable(t, lab.Storage(), "gpt-4o")
}

func TestStreamedFilterOnCapturesAndExperiments(t *testing.T) {
	lab, proxyURL := newProxySetup(t)
	for i := 0; i < 3; i++ {
		postProxy(t, proxyURL, chatBody("gpt-4o", false, false))
	}
	for i := 0; i < 2; i++ {
		postProxy(t, proxyURL, chatBody("gpt-4o", true, true))
	}
	waitForCaptures(t, lab.Storage(), 5)

	api := lab.GetAPIHandler()
	count := func(query string) int {
		t.Helper()
		rec := httptest.NewRecorder()
		api.ServeHTTP(rec, httptest.NewRequest("GET", "/replaylab/captures"+query, nil))
		if rec.Code != 200 {
			t.Fatalf("GET captures%s: %d %s", query, rec.Code, rec.Body.String())
		}
		var caps []replaylab.CaptureRecord
		_ = json.Unmarshal(rec.Body.Bytes(), &caps)
		return len(caps)
	}
	if n := count(""); n != 5 {
		t.Errorf("unfiltered = %d, want 5", n)
	}
	if n := count("?streamed=true"); n != 2 {
		t.Errorf("streamed=true = %d, want 2", n)
	}
	if n := count("?streamed=false"); n != 3 {
		t.Errorf("streamed=false = %d, want 3", n)
	}
	rec := httptest.NewRecorder()
	api.ServeHTTP(rec, httptest.NewRequest("GET", "/replaylab/captures?streamed=maybe", nil))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("streamed=maybe: %d, want 400", rec.Code)
	}

	dryRun := func(streamed string) replaylab.Experiment {
		t.Helper()
		body := `{"original_model":"gpt-4o","candidate_models":["gpt-4o-mini"],"sample_size":50,"dry_run":true` + streamed + `}`
		rec := httptest.NewRecorder()
		api.ServeHTTP(rec, httptest.NewRequest("POST", "/replaylab/experiments", strings.NewReader(body)))
		if rec.Code >= 300 {
			t.Fatalf("dry run %s: %d %s", body, rec.Code, rec.Body.String())
		}
		var exp replaylab.Experiment
		_ = json.Unmarshal(rec.Body.Bytes(), &exp)
		return exp
	}
	all, onlyStreamed, onlyPlain := dryRun(""), dryRun(`,"streamed":true`), dryRun(`,"streamed":false`)
	if all.SampleSize != 5 || onlyStreamed.SampleSize != 2 || onlyPlain.SampleSize != 3 {
		t.Errorf("sample sizes all=%d streamed=%d plain=%d, want 5/2/3", all.SampleSize, onlyStreamed.SampleSize, onlyPlain.SampleSize)
	}
	if onlyStreamed.Streamed == nil || !*onlyStreamed.Streamed {
		t.Error("experiment should record its streamed filter")
	}
	// Savings projections count all traffic, streamed included: 5 captures
	// in under a day project to 150 a month.
	if all.MonthlyRequestVolume != 150 || onlyStreamed.MonthlyRequestVolume != 150 {
		t.Errorf("monthly volume all=%d streamed=%d, want 150", all.MonthlyRequestVolume, onlyStreamed.MonthlyRequestVolume)
	}
}

func TestProxyStreamedCaptureReplaysToVerdict(t *testing.T) {
	lab, proxyURL := newProxySetup(t)
	for i := 0; i < 3; i++ {
		postProxy(t, proxyURL, chatBody("gpt-4o", true, true))
	}
	waitForCaptures(t, lab.Storage(), 3)

	streamed := true
	exp, err := lab.Engine().StartExperiment(replaylab.CreateExperimentRequest{
		OriginalModel: "gpt-4o", CandidateModels: []string{"gpt-4o-mini"}, SampleSize: 3, Streamed: &streamed,
	})
	if err != nil {
		t.Fatal(err)
	}
	final := waitTerminal(t, lab.Storage(), exp.ID)
	if final.Status != replaylab.StatusCompleted || len(final.Recommendations) != 1 {
		t.Fatalf("status=%s recommendations=%d", final.Status, len(final.Recommendations))
	}
	if r := final.Results[0]; r.SuccessfulRequests != 3 {
		t.Errorf("successful replays = %d, want 3", r.SuccessfulRequests)
	}
}
