package tests

// Streamed capture through the real Bifrost core: Replay Lab is registered as
// an LLM plugin and streamed chat requests go to the mock provider's SSE mode.

import (
	"context"
	"database/sql"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	bifrost "github.com/maximhq/bifrost/core"
	"github.com/maximhq/bifrost/core/schemas"
	_ "modernc.org/sqlite"

	"github.com/maximhq/bifrost/plugins/replaylab/internal/mockprovider"
	"github.com/maximhq/bifrost/plugins/replaylab/plugins/replaylab"
)

const streamPrompt = "Explain why atomic CPU operations avoid race conditions in concurrent code."

// streamResult is what the client saw on a streamed request.
type streamResult struct {
	text   string // concatenated content deltas
	chunks int
	err    *schemas.BifrostError
}

func chatRequest(model, prompt string) *schemas.BifrostChatRequest {
	return &schemas.BifrostChatRequest{
		Provider: schemas.OpenAI,
		Model:    model,
		Input: []schemas.ChatMessage{{
			Role:    schemas.ChatMessageRoleUser,
			Content: &schemas.ChatMessageContent{ContentStr: schemas.Ptr(prompt)},
		}},
	}
}

// streamChat sends a streamed chat request and reads the stream to its end.
// When stopAfterContent is true it cancels the request once the first content
// delta arrives, as a client that hangs up would.
func streamChat(t *testing.T, client *bifrost.Bifrost, model, prompt string, stopAfterContent bool) streamResult {
	t.Helper()
	ctx, cancel := schemas.NewBifrostContextWithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ch, berr := client.ChatCompletionStreamRequest(ctx, chatRequest(model, prompt))
	if berr != nil {
		return streamResult{err: berr}
	}
	var res streamResult
	var sb strings.Builder
	for chunk := range ch {
		res.chunks++
		if chunk.BifrostError != nil {
			res.err = chunk.BifrostError
			continue
		}
		if chunk.BifrostChatResponse == nil {
			continue
		}
		for _, c := range chunk.BifrostChatResponse.Choices {
			if c.ChatStreamResponseChoice != nil && c.ChatStreamResponseChoice.Delta != nil && c.ChatStreamResponseChoice.Delta.Content != nil {
				sb.WriteString(*c.ChatStreamResponseChoice.Delta.Content)
			}
		}
		if stopAfterContent && sb.Len() > 0 {
			cancel()
			stopAfterContent = false
		}
	}
	res.text = sb.String()
	return res
}

// nonStreamText returns the mock's non-streamed answer for the same request.
func nonStreamText(t *testing.T, client *bifrost.Bifrost, model, prompt string) string {
	t.Helper()
	ctx, cancel := schemas.NewBifrostContextWithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	resp, berr := client.ChatCompletionRequest(ctx, chatRequest(model, prompt))
	if berr != nil {
		t.Fatalf("non-streamed chat failed: %+v", berr.Error)
	}
	return *resp.Choices[0].Message.Content.ContentStr
}

func captureFor(t *testing.T, s replaylab.Storage, model string) *replaylab.CaptureRecord {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		caps, _ := s.GetCaptures(replaylab.CaptureFilter{Model: model, Limit: 10})
		if len(caps) > 0 {
			return caps[0]
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("no capture stored for %s", model)
	return nil
}

func assertNotReplayable(t *testing.T, s replaylab.Storage, model string) {
	t.Helper()
	caps, err := s.GetCaptures(replaylab.CaptureFilter{Model: model, Replayable: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(caps) != 0 {
		t.Fatalf("%d %s captures are replayable, want 0", len(caps), model)
	}
}

func metricsText(t *testing.T, lab *replaylab.Plugin) string {
	t.Helper()
	rec := httptest.NewRecorder()
	lab.GetAPIHandler().ServeHTTP(rec, httptest.NewRequest("GET", "/replaylab/metrics", nil))
	return rec.Body.String()
}

// setConfig changes runtime settings through PUT /replaylab/config.
func setConfig(t *testing.T, lab *replaylab.Plugin, body string) {
	t.Helper()
	rec := httptest.NewRecorder()
	lab.GetAPIHandler().ServeHTTP(rec, httptest.NewRequest("PUT", "/replaylab/config", strings.NewReader(body)))
	if rec.Code != 200 {
		t.Fatalf("PUT /replaylab/config %s: %d %s", body, rec.Code, rec.Body.String())
	}
}

func newStreamingSetup(t *testing.T) (*replaylab.Plugin, *bifrost.Bifrost) {
	t.Helper()
	mock := httptest.NewServer(mockprovider.Handler(mockprovider.Options{}))
	t.Cleanup(mock.Close)
	lab := newLab(t, mock.URL)
	return lab, startBifrost(t, mock.URL, lab)
}

func TestBifrostStreamCaptureReassemblesResponse(t *testing.T) {
	lab, client := newStreamingSetup(t)

	res := streamChat(t, client, "gpt-4o", streamPrompt, false)
	if res.err != nil {
		t.Fatalf("stream failed: %+v", res.err.Error)
	}
	if res.chunks < 3 {
		t.Fatalf("expected a multi-chunk stream, got %d chunks", res.chunks)
	}
	// The client sees exactly the answer a non-streamed request gets.
	want := nonStreamText(t, client, "gpt-4o", streamPrompt)
	if res.text != want {
		t.Fatalf("client stream text = %q, want %q", res.text, want)
	}

	caps := waitForCaptures(t, lab.Storage(), 2)
	var streamed, plain *replaylab.CaptureRecord
	for _, c := range caps {
		if c.IsStreaming {
			streamed = c
		} else {
			plain = c
		}
	}
	if streamed == nil || plain == nil {
		t.Fatalf("want one streamed and one non-streamed capture, got %+v", caps)
	}
	if streamed.ResponseJSON != res.text {
		t.Errorf("stored response = %q, want concatenated deltas %q", streamed.ResponseJSON, res.text)
	}
	if streamed.CompletionState != replaylab.CompletionStateComplete || streamed.StatusCode != 200 {
		t.Errorf("state=%s status=%d", streamed.CompletionState, streamed.StatusCode)
	}
	// Usage comes from the stream's final chunk (the mock reports len/4).
	if streamed.UsageEstimated || streamed.CompletionTokens != max(1, len(want)/4) || streamed.PromptTokens == 0 {
		t.Errorf("usage: estimated=%v prompt=%d completion=%d", streamed.UsageEstimated, streamed.PromptTokens, streamed.CompletionTokens)
	}
	if streamed.PromptTokens != plain.PromptTokens || streamed.CompletionTokens != plain.CompletionTokens {
		t.Errorf("streamed usage %d/%d differs from non-streamed %d/%d",
			streamed.PromptTokens, streamed.CompletionTokens, plain.PromptTokens, plain.CompletionTokens)
	}
	if streamed.TimeToFirstTokenMs == nil || *streamed.TimeToFirstTokenMs < 0 || *streamed.TimeToFirstTokenMs > streamed.LatencyMs {
		t.Errorf("time_to_first_token_ms = %v, latency_ms = %d", streamed.TimeToFirstTokenMs, streamed.LatencyMs)
	}
	if streamed.CostEstimate <= 0 {
		t.Errorf("cost estimate = %v", streamed.CostEstimate)
	}

	// Non-streamed captures are unchanged.
	if plain.CompletionState != replaylab.CompletionStateComplete || plain.TimeToFirstTokenMs != nil || plain.UsageEstimated {
		t.Errorf("non-streamed capture changed: state=%s ttft=%v estimated=%v", plain.CompletionState, plain.TimeToFirstTokenMs, plain.UsageEstimated)
	}
	if replayable, _ := lab.Storage().GetCaptures(replaylab.CaptureFilter{Model: "gpt-4o", Replayable: true}); len(replayable) != 2 {
		t.Errorf("replayable captures = %d, want 2", len(replayable))
	}
	if m := metricsText(t, lab); !strings.Contains(m, "bifrost_replaylab_stream_captures_total 1") {
		t.Errorf("streamed capture metric missing:\n%s", m)
	}
}

func TestBifrostStreamMidStreamErrorIsStoredAsError(t *testing.T) {
	lab, client := newStreamingSetup(t)
	model := "gpt-4o-stream-fail"

	res := streamChat(t, client, model, streamPrompt, false)
	if res.err == nil {
		t.Fatal("expected the stream to fail midway")
	}
	if res.text == "" {
		t.Fatal("expected some content before the failure")
	}

	c := captureFor(t, lab.Storage(), model)
	if c.CompletionState != replaylab.CompletionStateError {
		t.Errorf("completion_state = %s, want error", c.CompletionState)
	}
	if c.StatusCode < 400 || !strings.Contains(c.ErrorMessage, "mock stream failure") {
		t.Errorf("status=%d error=%q", c.StatusCode, c.ErrorMessage)
	}
	if c.ResponseJSON != res.text {
		t.Errorf("partial content = %q, want what arrived: %q", c.ResponseJSON, res.text)
	}
	assertNotReplayable(t, lab.Storage(), model)
}

func TestBifrostStreamAbandonedByClient(t *testing.T) {
	lab, client := newStreamingSetup(t)
	model := "gpt-4o-stream-stall"

	res := streamChat(t, client, model, streamPrompt, true)
	if res.text == "" {
		t.Fatal("expected some content before hanging up")
	}

	c := captureFor(t, lab.Storage(), model)
	if c.CompletionState != replaylab.CompletionStateAbandoned {
		t.Errorf("completion_state = %s, want abandoned", c.CompletionState)
	}
	if !strings.HasPrefix(res.text, c.ResponseJSON) || c.ResponseJSON == "" {
		t.Errorf("kept content %q is not the received prefix %q", c.ResponseJSON, res.text)
	}
	if c.StatusCode < 400 {
		t.Errorf("status = %d, want a failure status", c.StatusCode)
	}
	assertNotReplayable(t, lab.Storage(), model)
}

func TestBifrostStreamTruncatedAtCaptureCap(t *testing.T) {
	lab, client := newStreamingSetup(t)
	setConfig(t, lab, `{"max_stream_capture_bytes":24}`)

	res := streamChat(t, client, "gpt-4o", streamPrompt, false)
	if res.err != nil {
		t.Fatalf("stream failed: %+v", res.err.Error)
	}
	// The client still gets the whole answer.
	if want := nonStreamText(t, client, "gpt-4o", streamPrompt); res.text != want {
		t.Fatalf("client text = %q, want %q", res.text, want)
	}

	caps := waitForCaptures(t, lab.Storage(), 2)
	var c *replaylab.CaptureRecord
	for _, x := range caps {
		if x.IsStreaming {
			c = x
		}
	}
	if c == nil {
		t.Fatal("no streamed capture")
	}
	if c.CompletionState != replaylab.CompletionStateTruncated {
		t.Errorf("completion_state = %s, want truncated", c.CompletionState)
	}
	if len(c.ResponseJSON) > 24 || !strings.HasPrefix(res.text, c.ResponseJSON) {
		t.Errorf("stored %d bytes %q, want a prefix of at most 24 bytes", len(c.ResponseJSON), c.ResponseJSON)
	}
	if c.StatusCode != 200 || c.UsageEstimated {
		t.Errorf("status=%d estimated=%v; usage should still come from the final chunk", c.StatusCode, c.UsageEstimated)
	}
	replayable, _ := lab.Storage().GetCaptures(replaylab.CaptureFilter{Model: "gpt-4o", Replayable: true})
	if len(replayable) != 1 || replayable[0].IsStreaming {
		t.Errorf("only the non-streamed capture should be replayable, got %d", len(replayable))
	}
	if m := metricsText(t, lab); !strings.Contains(m, "bifrost_replaylab_stream_captures_truncated_total 1") {
		t.Errorf("truncation metric missing:\n%s", m)
	}
}

func TestBifrostStreamCaptureFollowsPrivacyAndSampling(t *testing.T) {
	t.Run("hash only", func(t *testing.T) {
		lab, client := newStreamingSetup(t)
		setConfig(t, lab, `{"store_hashes_only":true}`)
		res := streamChat(t, client, "gpt-4o", streamPrompt, false)
		c := captureFor(t, lab.Storage(), "gpt-4o")
		if !c.HashOnly || c.ResponseJSON != replaylab.HashString(res.text) {
			t.Errorf("hash_only=%v response=%q", c.HashOnly, c.ResponseJSON)
		}
	})
	t.Run("sampling off", func(t *testing.T) {
		lab, client := newStreamingSetup(t)
		setConfig(t, lab, `{"sampling_rate":0}`)
		streamChat(t, client, "gpt-4o", streamPrompt, false)
		time.Sleep(400 * time.Millisecond)
		if n, _ := lab.Storage().GetCaptureCount(); n != 0 {
			t.Errorf("captured %d streams with sampling at 0", n)
		}
	})
	t.Run("redaction of streamed content", func(t *testing.T) {
		lab := newLab(t, "http://127.0.0.1:1")
		if !lab.RecordStreamRequest("s-1", "", "", "openai", "gpt-4o", `[{"role":"user","content":"who?"}]`, "", "") {
			t.Fatal("stream not captured")
		}
		// The address is split across deltas, so only the reassembled text shows it.
		for _, d := range []string{"Write to bob.smith", "@example", ".com today"} {
			lab.AppendStreamContent("s-1", d)
		}
		lab.FinishStream("s-1", replaylab.StreamOutcome{State: replaylab.CompletionStateComplete})
		c := captureFor(t, lab.Storage(), "gpt-4o")
		if strings.Contains(c.ResponseJSON, "bob.smith@example.com") || !strings.Contains(c.ResponseJSON, "[REDACTED_EMAIL]") {
			t.Errorf("streamed content not redacted: %q", c.ResponseJSON)
		}
		if !c.UsageEstimated || c.CompletionTokens == 0 {
			t.Errorf("missing usage should be estimated: estimated=%v completion=%d", c.UsageEstimated, c.CompletionTokens)
		}
	})
}

func TestStreamedCapturesReplayToVerdict(t *testing.T) {
	// Simulated latency keeps the latency comparison meaningful; with none,
	// millisecond noise can read as a >40% regression and flip the verdict.
	mock := httptest.NewServer(mockprovider.Handler(mockprovider.Options{LatencyScale: 0.25}))
	t.Cleanup(mock.Close)
	lab := newLab(t, mock.URL)
	client := startBifrost(t, mock.URL, lab)
	prompts := []string{
		"Extract the structured JSON object containing name, email, and company from: 'Bo, bo@x.io, Acme'",
		"Explain why atomic CPU operations avoid race conditions in concurrent code.",
		"Summarize the advantages of micro-frontends versus monolithic single page apps.",
	}
	for i := 0; i < 24; i++ {
		if res := streamChat(t, client, "gpt-4o", prompts[i%len(prompts)], false); res.err != nil {
			t.Fatalf("stream %d failed: %+v", i, res.err.Error)
		}
	}
	waitForCaptures(t, lab.Storage(), 24)

	exp, err := lab.Engine().StartExperiment(replaylab.CreateExperimentRequest{
		OriginalModel: "gpt-4o", CandidateModels: []string{"gpt-4o-mini", "meta-llama-3.1-8b"}, SampleSize: 24,
	})
	if err != nil {
		t.Fatal(err)
	}
	final := waitTerminal(t, lab.Storage(), exp.ID)
	if final.Status != replaylab.StatusCompleted || final.TotalReplays != 48 {
		t.Fatalf("status=%s total_replays=%d", final.Status, final.TotalReplays)
	}
	byModel := map[string]string{}
	for _, r := range final.Recommendations {
		byModel[r.CandidateModel] = r.Verdict
	}
	if byModel["gpt-4o-mini"] != replaylab.VerdictSafeToSwitch {
		t.Errorf("gpt-4o-mini verdict = %s, want SAFE_TO_SWITCH", byModel["gpt-4o-mini"])
	}
	if byModel["meta-llama-3.1-8b"] != replaylab.VerdictDoNotSwitch {
		t.Errorf("llama-8b verdict = %s, want DO_NOT_SWITCH", byModel["meta-llama-3.1-8b"])
	}
}

func TestCapturesFromOlderDatabasesReadAsComplete(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	// The captures table as it was before streamed capture.
	for _, q := range []string{
		`CREATE TABLE captures (id TEXT PRIMARY KEY, request_id TEXT NOT NULL, timestamp DATETIME NOT NULL,
			virtual_key TEXT, tenant TEXT, provider TEXT NOT NULL, model TEXT NOT NULL, messages_json TEXT NOT NULL,
			params_json TEXT, response_json TEXT, prompt_tokens INTEGER, completion_tokens INTEGER, total_tokens INTEGER,
			latency_ms INTEGER, cost_estimate REAL, status_code INTEGER, error_message TEXT, is_streaming BOOLEAN,
			tag TEXT, hash_only BOOLEAN)`,
		`INSERT INTO captures VALUES ('old-1','r',CURRENT_TIMESTAMP,'','','openai','gpt-4o','[]','','answer',
			10,10,20,100,0.001,200,'',0,'',0)`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()

	cfg := replaylab.DefaultConfig()
	cfg.DBPath = path
	lab, err := replaylab.NewPlugin(cfg)
	if err != nil {
		t.Fatalf("NewPlugin on an old database: %v", err)
	}
	defer lab.Cleanup()
	c, err := lab.Storage().GetCaptureByID("old-1")
	if err != nil || c == nil {
		t.Fatalf("read old capture: %v", err)
	}
	if c.CompletionState != replaylab.CompletionStateComplete || c.TimeToFirstTokenMs != nil || c.UsageEstimated {
		t.Errorf("old capture reads as state=%s ttft=%v estimated=%v", c.CompletionState, c.TimeToFirstTokenMs, c.UsageEstimated)
	}
	if caps, _ := lab.Storage().GetCaptures(replaylab.CaptureFilter{Replayable: true}); len(caps) != 1 {
		t.Errorf("old capture should stay replayable, got %d", len(caps))
	}
}
