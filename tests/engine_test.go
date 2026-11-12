package tests

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/maximhq/bifrost/plugins/replaylab/internal/mockprovider"
	"github.com/maximhq/bifrost/plugins/replaylab/plugins/replaylab"
)

// newLab starts a plugin pointed at upstream and returns it with cleanup registered.
func newLab(t *testing.T, upstream string) *replaylab.Plugin {
	t.Helper()
	cfg := replaylab.DefaultConfig()
	cfg.DBPath = filepath.Join(t.TempDir(), "lab.db")
	cfg.GatewayURL = upstream
	p, err := replaylab.NewPlugin(cfg)
	if err != nil {
		t.Fatalf("NewPlugin: %v", err)
	}
	t.Cleanup(func() { _ = p.Cleanup() })
	return p
}

func seed(t *testing.T, s replaylab.Storage, n int, model string) {
	t.Helper()
	recs := make([]*replaylab.CaptureRecord, n)
	for i := range recs {
		recs[i] = &replaylab.CaptureRecord{
			RequestID:        "r",
			Timestamp:        time.Now().Add(-time.Duration(i) * time.Minute),
			Provider:         "openai",
			Model:            model,
			MessagesJSON:     `[{"role":"user","content":"Explain atomic operations"}]`,
			ResponseJSON:     "Atomic operations complete without interruption.",
			PromptTokens:     1000,
			CompletionTokens: 1000,
			LatencyMs:        200,
			CostEstimate:     replaylab.EstimateCost(model, 1000, 1000),
			StatusCode:       200,
		}
		recs[i].ID = fmt.Sprintf("cap-%s-%d", model, i)
	}
	if err := s.SaveCapturesBatch(recs); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

func waitTerminal(t *testing.T, s replaylab.Storage, id string) *replaylab.Experiment {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		exp, _ := s.GetExperiment(id)
		if exp != nil && replaylab.IsTerminalStatus(exp.Status) {
			return exp
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("experiment %s did not finish", id)
	return nil
}

// heavyUpstream answers every chat request with 1000 prompt + 1000 completion tokens.
func heavyUpstream() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"Atomic operations complete without interruption."}}],"usage":{"prompt_tokens":1000,"completion_tokens":1000}}`))
	}))
}

func TestBudgetCapStopsBeforeOverspendAndKeepsPartialResults(t *testing.T) {
	mock := heavyUpstream()
	defer mock.Close()
	lab := newLab(t, mock.URL)
	seed(t, lab.Storage(), 20, "gpt-4o")

	// Each gpt-4o replay of 1000+1000 tokens is estimated at $0.0125.
	exp, err := lab.Engine().StartExperiment(replaylab.CreateExperimentRequest{
		OriginalModel: "gpt-4o", CandidateModels: []string{"gpt-4o"}, SampleSize: 20, BudgetCapDollars: 0.05,
	})
	if err != nil {
		t.Fatal(err)
	}
	final := waitTerminal(t, lab.Storage(), exp.ID)
	if final.Status != replaylab.StatusBudgetExhausted {
		t.Fatalf("status = %s, want %s", final.Status, replaylab.StatusBudgetExhausted)
	}
	if final.SpentDollars > final.BudgetCapDollars {
		t.Fatalf("spent $%.4f exceeds cap $%.4f", final.SpentDollars, final.BudgetCapDollars)
	}
	if len(final.Results) != 1 || final.Results[0].TotalRequests == 0 {
		t.Fatalf("expected partial results to be kept, got %+v", final.Results)
	}
	if final.CompletedAt == nil {
		t.Fatal("expected CompletedAt on a budget-stopped experiment")
	}
}

func TestUpstreamHTTPErrorsCountAsFailures(t *testing.T) {
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"boom"}`, http.StatusInternalServerError)
	}))
	defer failing.Close()
	lab := newLab(t, failing.URL)
	seed(t, lab.Storage(), 5, "gpt-4o")

	exp, err := lab.Engine().StartExperiment(replaylab.CreateExperimentRequest{
		OriginalModel: "gpt-4o", CandidateModels: []string{"gpt-4o-mini"}, SampleSize: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	final := waitTerminal(t, lab.Storage(), exp.ID)
	r := final.Results[0]
	if r.FailedRequests != 5 || r.SuccessfulRequests != 0 {
		t.Fatalf("failed=%d successful=%d, want 5/0", r.FailedRequests, r.SuccessfulRequests)
	}
	if final.Recommendations[0].Verdict != replaylab.VerdictDoNotSwitch {
		t.Fatalf("verdict = %s, want DO_NOT_SWITCH", final.Recommendations[0].Verdict)
	}
}

func TestProxyCapturesRedactedTrafficAndParams(t *testing.T) {
	mock := httptest.NewServer(mockprovider.Handler(mockprovider.Options{}))
	defer mock.Close()
	lab := newLab(t, mock.URL)
	proxy := httptest.NewServer(lab.ProxyHandler())
	defer proxy.Close()

	body := `{"model":"gpt-4o","max_tokens":64,"messages":[{"role":"user","content":"Email bob@example.com about JSON"}]}`
	req, _ := http.NewRequest(http.MethodPost, proxy.URL+"/v1/chat/completions", strings.NewReader(body))
	req.Header.Set(replaylab.HeaderTag, "support")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("proxy status %d", resp.StatusCode)
	}

	var caps []*replaylab.CaptureRecord
	for i := 0; i < 50 && len(caps) == 0; i++ {
		time.Sleep(20 * time.Millisecond)
		caps, _ = lab.Storage().GetCaptures(replaylab.CaptureFilter{Tag: "support"})
	}
	if len(caps) != 1 {
		t.Fatalf("expected 1 capture, got %d", len(caps))
	}
	c := caps[0]
	if strings.Contains(c.MessagesJSON, "bob@example.com") {
		t.Errorf("email not redacted: %s", c.MessagesJSON)
	}
	if !strings.Contains(c.ParamsJSON, "max_tokens") {
		t.Errorf("params not captured: %q", c.ParamsJSON)
	}
	if c.ResponseJSON == "" || c.PromptTokens == 0 {
		t.Errorf("response content/usage not captured: %+v", c)
	}

	// Replays are marked and must not be captured again.
	req, _ = http.NewRequest(http.MethodPost, proxy.URL+"/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("X-Replaylab-Replay", "1")
	resp, _ = http.DefaultClient.Do(req)
	resp.Body.Close()
	time.Sleep(400 * time.Millisecond)
	if n, _ := lab.Storage().GetCaptureCount(); n != 1 {
		t.Errorf("replay request was captured: count=%d", n)
	}
}

func TestSSEStreamEndsWithFinalEvent(t *testing.T) {
	mock := httptest.NewServer(mockprovider.Handler(mockprovider.Options{}))
	defer mock.Close()
	lab := newLab(t, mock.URL)
	seed(t, lab.Storage(), 3, "gpt-4o")
	api := httptest.NewServer(lab.GetAPIHandler())
	defer api.Close()

	payload, _ := json.Marshal(map[string]any{"original_model": "gpt-4o", "candidate_models": []string{"gpt-4o-mini"}, "sample_size": 3})
	resp, err := http.Post(api.URL+"/replaylab/experiments", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	var exp replaylab.Experiment
	_ = json.NewDecoder(resp.Body).Decode(&exp)
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("create status %d", resp.StatusCode)
	}

	// Subscribing after completion must still terminate with a final event.
	waitTerminal(t, lab.Storage(), exp.ID)
	client := &http.Client{Timeout: 5 * time.Second}
	stream, err := client.Get(api.URL + "/replaylab/experiments/" + exp.ID + "/events")
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Body.Close()
	var last replaylab.ReplayEvent
	sc := bufio.NewScanner(stream.Body)
	for sc.Scan() {
		if data, ok := strings.CutPrefix(sc.Text(), "data: "); ok {
			_ = json.Unmarshal([]byte(data), &last)
		}
	}
	if !last.Final || last.Status != replaylab.StatusCompleted {
		t.Fatalf("last event = %+v, want final completed event", last)
	}
}

func TestCreateExperimentValidation(t *testing.T) {
	lab := newLab(t, "http://127.0.0.1:1")
	api := httptest.NewServer(lab.GetAPIHandler())
	defer api.Close()

	for name, body := range map[string]string{
		"no candidates":   `{"original_model":"gpt-4o","candidate_models":[]}`,
		"negative budget": `{"candidate_models":["x"],"budget_cap_dollars":-1}`,
		"unknown field":   `{"candidate_models":["x"],"bogus":true}`,
	} {
		resp, err := http.Post(api.URL+"/replaylab/experiments", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", name, resp.StatusCode)
		}
	}
}

func TestConfigUpdateTakesEffect(t *testing.T) {
	lab := newLab(t, "http://127.0.0.1:1")
	api := httptest.NewServer(lab.GetAPIHandler())
	defer api.Close()

	req, _ := http.NewRequest(http.MethodPut, api.URL+"/replaylab/config", strings.NewReader(`{"enabled":false}`))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	lab.RecordRequest("r1", "vk", "", "openai", "gpt-4o", `[]`, "")
	lab.RecordResponse("r1", "ok", 1, 1, 200, "", false)
	time.Sleep(400 * time.Millisecond)
	if n, _ := lab.Storage().GetCaptureCount(); n != 0 {
		t.Fatalf("capture recorded while disabled: %d", n)
	}

	req, _ = http.NewRequest(http.MethodPut, api.URL+"/replaylab/config", strings.NewReader(`{"sampling_rate":2}`))
	resp, _ = http.DefaultClient.Do(req)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid sampling rate accepted: %d", resp.StatusCode)
	}
}

func TestPricingLookupHandlesPrefixesAndSnapshots(t *testing.T) {
	cases := map[string]float64{
		"gpt-4o":                 2.50,
		"openai/gpt-4o-mini":     0.15,
		"gpt-4o-2024-08-06":      2.50,
		"gpt-4o-mini-2024-07-18": 0.15,
	}
	for model, wantIn := range cases {
		p, known := replaylab.LookupPricing(model)
		if !known || p.InputPer1M != wantIn {
			t.Errorf("%s: got %+v known=%v, want input %.2f", model, p, known, wantIn)
		}
	}
	if _, known := replaylab.LookupPricing("totally-unknown"); known {
		t.Error("unknown model reported as known")
	}
}

func TestTrafficVolumeMonthlyEstimate(t *testing.T) {
	lab := newLab(t, "http://127.0.0.1:1")
	seed(t, lab.Storage(), 10, "gpt-4o")
	vol, err := lab.Storage().GetTrafficVolume("gpt-4o")
	if err != nil {
		t.Fatal(err)
	}
	if vol.Count != 10 || vol.First.IsZero() || vol.Last.IsZero() {
		t.Fatalf("unexpected volume %+v", vol)
	}
	// 10 captures within a day extrapolate to 10/day * 30.
	if got := vol.MonthlyEstimate(); got != 300 {
		t.Fatalf("monthly estimate = %d, want 300", got)
	}
}
