package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/maximhq/bifrost/plugins/replaylab/internal/mockprovider"
	"github.com/maximhq/bifrost/plugins/replaylab/plugins/replaylab"
)

const baselineAnswer = "Atomic operations complete without interruption."

// judgeUpstream answers candidate models with fixed content and sends every
// other request (the judge) to the deterministic mock provider. intercept,
// when set, may answer a judge request itself by returning true.
func judgeUpstream(t *testing.T, candidates map[string]string, intercept func(w http.ResponseWriter, body []byte) bool) *httptest.Server {
	t.Helper()
	mock := mockprovider.Handler(mockprovider.Options{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Model string `json:"model"`
		}
		_ = json.Unmarshal(body, &req)
		if content, ok := candidates[req.Model]; ok {
			w.Header().Set("Content-Type", "application/json")
			b, _ := json.Marshal(map[string]any{
				"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": content}}},
				"usage":   map[string]int{"prompt_tokens": 1000, "completion_tokens": 1000},
			})
			_, _ = w.Write(b)
			return
		}
		if intercept != nil && intercept(w, body) {
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		mock.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// seedPrompts stores one gpt-4o capture per prompt, all with the same baseline answer.
func seedPrompts(t *testing.T, s replaylab.Storage, prompts []string) {
	t.Helper()
	recs := make([]*replaylab.CaptureRecord, len(prompts))
	for i, p := range prompts {
		msgs, _ := json.Marshal([]map[string]string{{"role": "user", "content": p}})
		recs[i] = &replaylab.CaptureRecord{
			ID:               fmt.Sprintf("cap-%d", i),
			RequestID:        "r",
			Timestamp:        time.Now().Add(-time.Duration(i) * time.Minute),
			Provider:         "openai",
			Model:            "gpt-4o",
			MessagesJSON:     string(msgs),
			ResponseJSON:     baselineAnswer,
			PromptTokens:     1000,
			CompletionTokens: 1000,
			LatencyMs:        200,
			CostEstimate:     replaylab.EstimateCost("gpt-4o", 1000, 1000),
			StatusCode:       200,
		}
	}
	if err := s.SaveCapturesBatch(recs); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

func runExperiment(t *testing.T, lab *replaylab.Plugin, req replaylab.CreateExperimentRequest) *replaylab.Experiment {
	t.Helper()
	exp, err := lab.Engine().StartExperiment(req)
	if err != nil {
		t.Fatal(err)
	}
	final := waitTerminal(t, lab.Storage(), exp.ID)
	if final.Status != replaylab.StatusCompleted {
		t.Fatalf("status = %s (%s), want completed", final.Status, final.StopReason)
	}
	if len(final.Results) != 1 || len(final.Recommendations) != 1 {
		t.Fatalf("want one result, got %+v", final.Results)
	}
	return final
}

func TestJudgeRatesEquivalentRewordingSafe(t *testing.T) {
	up := judgeUpstream(t, map[string]string{"gpt-4o-mini": "Indivisible steps finish with no preemption."}, nil)
	lab := newLab(t, up.URL)
	seed(t, lab.Storage(), 20, "gpt-4o")
	req := replaylab.CreateExperimentRequest{OriginalModel: "gpt-4o", CandidateModels: []string{"gpt-4o-mini"}, SampleSize: 20}

	unjudged := runExperiment(t, lab, req)
	if v := unjudged.Recommendations[0].Verdict; v == replaylab.VerdictSafeToSwitch {
		t.Fatalf("similarity alone rated the rewording %s; the test needs a candidate similarity misjudges", v)
	}

	req.Judge = &replaylab.JudgeConfig{Model: "judge-mock"}
	judged := runExperiment(t, lab, req)
	r := judged.Results[0]
	if v := judged.Recommendations[0].Verdict; v != replaylab.VerdictSafeToSwitch {
		t.Fatalf("verdict = %s (%s), want SAFE_TO_SWITCH", v, r.Reasoning)
	}
	if r.JudgeScore == nil || *r.JudgeScore != 100 {
		t.Fatalf("judge_score = %v, want 100", r.JudgeScore)
	}
	if *r.JudgeCoverage != 100 || *r.JudgeDisagreementRate != 0 {
		t.Fatalf("coverage=%v disagreement=%v, want 100/0", *r.JudgeCoverage, *r.JudgeDisagreementRate)
	}
	if got := *r.JudgeWins + *r.JudgeTies + *r.JudgeLosses; got != 2*r.SuccessfulRequests {
		t.Fatalf("W+T+L = %d, want two judgements per replay (%d)", got, 2*r.SuccessfulRequests)
	}
	if r.AvgQualityScore >= replaylab.QualitySafeThreshold {
		t.Fatalf("similarity score %.1f should still be reported as low", r.AvgQualityScore)
	}
	if judged.SpentDollars <= unjudged.SpentDollars {
		t.Fatalf("judge spend not counted: judged $%.4f vs unjudged $%.4f", judged.SpentDollars, unjudged.SpentDollars)
	}
}

func TestBiasedJudgeIsNeutralisedByTwoOrders(t *testing.T) {
	up := judgeUpstream(t, map[string]string{"gpt-4o-mini": baselineAnswer}, nil)
	lab := newLab(t, up.URL)
	seed(t, lab.Storage(), 20, "gpt-4o")

	final := runExperiment(t, lab, replaylab.CreateExperimentRequest{
		OriginalModel: "gpt-4o", CandidateModels: []string{"gpt-4o-mini"}, SampleSize: 20,
		Judge: &replaylab.JudgeConfig{Model: "judge-biased"},
	})
	r := final.Results[0]
	if r.JudgeScore == nil || *r.JudgeScore != 50 {
		t.Fatalf("judge_score = %v, want 50", r.JudgeScore)
	}
	if *r.JudgeDisagreementRate <= replaylab.MaxJudgeDisagreementPercent {
		t.Fatalf("disagreement = %.0f%%, want high", *r.JudgeDisagreementRate)
	}
	if *r.JudgeWins != *r.JudgeLosses || *r.JudgeTies != 0 {
		t.Fatalf("W/T/L = %d/%d/%d, want equal wins and losses", *r.JudgeWins, *r.JudgeTies, *r.JudgeLosses)
	}
	if v := final.Recommendations[0].Verdict; v == replaylab.VerdictSafeToSwitch {
		t.Fatalf("verdict = %s with an inconsistent judge", v)
	}
}

func TestJudgeFailuresFallBackAndCapVerdict(t *testing.T) {
	prompts := make([]string, 20)
	for i := range prompts {
		switch {
		case i%4 == 0:
			prompts[i] = fmt.Sprintf("Explain atomic operations, case %d FLAKY-ERROR", i)
		case i%4 == 1:
			prompts[i] = fmt.Sprintf("Explain atomic operations, case %d FLAKY-GARBAGE", i)
		default:
			prompts[i] = fmt.Sprintf("Explain atomic operations, case %d", i)
		}
	}
	up := judgeUpstream(t, map[string]string{"gpt-4o-mini": baselineAnswer}, func(w http.ResponseWriter, body []byte) bool {
		switch {
		case bytes.Contains(body, []byte("FLAKY-ERROR")):
			http.Error(w, `{"error":"judge overloaded"}`, http.StatusServiceUnavailable)
			return true
		case bytes.Contains(body, []byte("FLAKY-GARBAGE")):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"I think the first one is nicer."}}]}`))
			return true
		}
		return false
	})
	lab := newLab(t, up.URL)
	seedPrompts(t, lab.Storage(), prompts)

	final := runExperiment(t, lab, replaylab.CreateExperimentRequest{
		OriginalModel: "gpt-4o", CandidateModels: []string{"gpt-4o-mini"}, SampleSize: 20,
		Judge: &replaylab.JudgeConfig{Model: "judge-mock"},
	})
	r := final.Results[0]
	if r.SuccessfulRequests != 20 || r.FailedRequests != 0 {
		t.Fatalf("judge failures must not fail replays: successful=%d failed=%d", r.SuccessfulRequests, r.FailedRequests)
	}
	if *r.JudgeCoverage != 50 {
		t.Fatalf("judge_coverage = %v, want 50", *r.JudgeCoverage)
	}
	if r.JudgeScore == nil || *r.JudgeScore != 100 {
		t.Fatalf("judge_score = %v, want 100 over the judged replays", r.JudgeScore)
	}
	if r.AvgQualityScore != 100 {
		t.Fatalf("similarity score = %.1f, want 100 kept for every replay", r.AvgQualityScore)
	}
	if v := final.Recommendations[0].Verdict; v != replaylab.VerdictSwitchWithCaution {
		t.Fatalf("verdict = %s (%s), want low coverage to cap at SWITCH_WITH_CAUTION", v, r.Reasoning)
	}
}

func TestJudgeFieldsOmittedWithoutJudge(t *testing.T) {
	mock := httptest.NewServer(mockprovider.Handler(mockprovider.Options{}))
	defer mock.Close()
	lab := newLab(t, mock.URL)
	seed(t, lab.Storage(), 5, "gpt-4o")
	api := httptest.NewServer(lab.GetAPIHandler())
	defer api.Close()

	get := func(body string) string {
		t.Helper()
		resp, err := http.Post(api.URL+"/replaylab/experiments", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		var exp replaylab.Experiment
		_ = json.NewDecoder(resp.Body).Decode(&exp)
		resp.Body.Close()
		if resp.StatusCode != http.StatusAccepted {
			t.Fatalf("create status %d", resp.StatusCode)
		}
		waitTerminal(t, lab.Storage(), exp.ID)
		resp, err = http.Get(api.URL + "/replaylab/experiments/" + exp.ID)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		return string(raw)
	}

	plain := get(`{"original_model":"gpt-4o","candidate_models":["gpt-4o-mini"],"sample_size":5}`)
	if strings.Contains(plain, `"judge`) {
		t.Fatalf("experiment without a judge exposes judge fields: %s", plain)
	}
	judged := get(`{"original_model":"gpt-4o","candidate_models":["gpt-4o-mini"],"sample_size":5,"judge":{"model":"judge-mock"}}`)
	for _, field := range []string{`"judge":{"model":"judge-mock"`, `"rubric":`, `"judge_spent_dollars"`, `"judge_samples"`, `"judge_score"`, `"judge_wins"`, `"judge_ties"`, `"judge_losses"`, `"judge_coverage"`, `"judge_disagreement_rate"`} {
		if !strings.Contains(judged, field) {
			t.Errorf("judged experiment is missing %s", field)
		}
	}
}

func TestCreateExperimentRejectsInvalidJudge(t *testing.T) {
	lab := newLab(t, "http://127.0.0.1:1")
	api := httptest.NewServer(lab.GetAPIHandler())
	defer api.Close()

	for name, body := range map[string]string{
		"judge is a candidate": `{"candidate_models":["gpt-4o-mini","gpt-4o"],"judge":{"model":" gpt-4o "}}`,
		"judge without model":  `{"candidate_models":["gpt-4o-mini"],"judge":{}}`,
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
