package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/maximhq/bifrost/plugins/replaylab/plugins/replaylab"
)

// startAndWait runs an experiment to any terminal state.
func startAndWait(t *testing.T, lab *replaylab.Plugin, req replaylab.CreateExperimentRequest) *replaylab.Experiment {
	t.Helper()
	exp, err := lab.Engine().StartExperiment(req)
	if err != nil {
		t.Fatal(err)
	}
	return waitTerminal(t, lab.Storage(), exp.ID)
}

func TestJudgeCostsKeepTheBudgetCapHard(t *testing.T) {
	up := judgeUpstream(t, map[string]string{"gpt-4o-mini": baselineAnswer}, nil)
	lab := newLab(t, up.URL)
	seed(t, lab.Storage(), 20, "gpt-4o")
	req := replaylab.CreateExperimentRequest{OriginalModel: "gpt-4o", CandidateModels: []string{"gpt-4o-mini"}, SampleSize: 20, BudgetCapDollars: 0.5}

	if plain := startAndWait(t, lab, req); plain.Status != replaylab.StatusCompleted {
		t.Fatalf("without a judge the run should fit the cap: %s (%s)", plain.Status, plain.StopReason)
	}

	req.Judge = &replaylab.JudgeConfig{Model: "gpt-4"} // expensive, known price
	final := startAndWait(t, lab, req)
	if final.Status != replaylab.StatusBudgetExhausted {
		t.Fatalf("status = %s, want budget_exhausted once judge costs count", final.Status)
	}
	if final.SpentDollars > final.BudgetCapDollars {
		t.Fatalf("spent $%.4f, above the $%.2f cap", final.SpentDollars, final.BudgetCapDollars)
	}
	if final.JudgeSpentDollars == nil || *final.JudgeSpentDollars <= 0 {
		t.Fatalf("judge_spent_dollars = %v, want positive", final.JudgeSpentDollars)
	}
	r := final.Results[0]
	if r.JudgeSpentDollars == nil || math.Abs(*r.JudgeSpentDollars-*final.JudgeSpentDollars) > 1e-12 {
		t.Fatalf("candidate judge_spent_dollars = %v, want the experiment's $%.6f", r.JudgeSpentDollars, *final.JudgeSpentDollars)
	}
	if *final.JudgeSpentDollars >= final.SpentDollars {
		t.Fatalf("judge spend $%.6f should be part of total spend $%.6f", *final.JudgeSpentDollars, final.SpentDollars)
	}
	if final.JudgePriceEstimated {
		t.Fatal("gpt-4 has a known price; judge cost must not be flagged as estimated")
	}
}

// Captures whose token counts understate their text: the pre-replay
// projection fits the cap, so only the per-call judge guard can stop overspend.
func TestJudgeCallIsSkippedWhenItCouldCrossTheCap(t *testing.T) {
	up := judgeUpstream(t, map[string]string{"gpt-4o-mini": baselineAnswer}, nil)
	lab := newLab(t, up.URL)
	long := strings.Repeat("Explain atomic operations in great detail. ", 500)
	msgs, _ := json.Marshal([]map[string]string{{"role": "user", "content": long}})
	recs := make([]*replaylab.CaptureRecord, 3)
	for i := range recs {
		recs[i] = &replaylab.CaptureRecord{
			ID: fmt.Sprintf("cap-long-%d", i), RequestID: "r", Timestamp: time.Now().Add(-time.Duration(i) * time.Minute),
			Provider: "openai", Model: "gpt-4o", MessagesJSON: string(msgs), ResponseJSON: baselineAnswer,
			PromptTokens: 10, CompletionTokens: 10, LatencyMs: 200, StatusCode: 200,
		}
	}
	if err := lab.Storage().SaveCapturesBatch(recs); err != nil {
		t.Fatal(err)
	}

	final := startAndWait(t, lab, replaylab.CreateExperimentRequest{
		OriginalModel: "gpt-4o", CandidateModels: []string{"gpt-4o-mini"}, SampleSize: 3, BudgetCapDollars: 0.1,
		Judge: &replaylab.JudgeConfig{Model: "gpt-4"},
	})
	if final.Status != replaylab.StatusBudgetExhausted {
		t.Fatalf("status = %s, want budget_exhausted", final.Status)
	}
	if final.SpentDollars > final.BudgetCapDollars {
		t.Fatalf("spent $%.4f, above the $%.2f cap", final.SpentDollars, final.BudgetCapDollars)
	}
	if r := final.Results[0]; r.SuccessfulRequests != 1 || *r.JudgeCoverage != 0 {
		t.Fatalf("want the one landed replay kept unjudged, got successful=%d coverage=%v", r.SuccessfulRequests, *r.JudgeCoverage)
	}
}

func TestDryRunEstimatesJudgeCostSeparately(t *testing.T) {
	lab := newLab(t, "http://127.0.0.1:1") // a dry run must not call any model
	seed(t, lab.Storage(), 20, "gpt-4o")
	base := replaylab.CreateExperimentRequest{OriginalModel: "gpt-4o", CandidateModels: []string{"gpt-4o-mini"}, SampleSize: 20, DryRun: true}

	plain, err := lab.Engine().StartExperiment(base)
	if err != nil {
		t.Fatal(err)
	}
	if plain.EstimatedJudgeDollars != nil || plain.EstimatedReplayDollars != nil {
		t.Fatal("dry run without a judge must not report a judge estimate")
	}

	judged := base
	judged.Judge = &replaylab.JudgeConfig{Model: "gpt-4o"}
	all, err := lab.Engine().StartExperiment(judged)
	if err != nil {
		t.Fatal(err)
	}
	if all.EstimatedJudgeDollars == nil || *all.EstimatedJudgeDollars <= 0 {
		t.Fatalf("estimated_judge_dollars = %v, want positive", all.EstimatedJudgeDollars)
	}
	if math.Abs(*all.EstimatedReplayDollars-plain.SpentDollars) > 1e-12 {
		t.Fatalf("replay estimate $%.6f should match the unjudged estimate $%.6f", *all.EstimatedReplayDollars, plain.SpentDollars)
	}
	if math.Abs(*all.EstimatedReplayDollars+*all.EstimatedJudgeDollars-all.SpentDollars) > 1e-12 {
		t.Fatalf("replay + judge estimates should sum to spent_dollars")
	}

	limited := base
	limited.Judge = &replaylab.JudgeConfig{Model: "gpt-4o", MaxJudgeSamples: 5}
	part, err := lab.Engine().StartExperiment(limited)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := *part.EstimatedJudgeDollars, *all.EstimatedJudgeDollars/4; math.Abs(got-want) > 1e-9 {
		t.Fatalf("judge estimate with max_judge_samples=5 of 20 = $%.6f, want $%.6f", got, want)
	}
}

func TestMaxJudgeSamplesLimitsJudgedReplays(t *testing.T) {
	up := judgeUpstream(t, map[string]string{"gpt-4o-mini": "Indivisible steps finish with no preemption."}, nil)
	lab := newLab(t, up.URL)
	seed(t, lab.Storage(), 20, "gpt-4o")

	final := runExperiment(t, lab, replaylab.CreateExperimentRequest{
		OriginalModel: "gpt-4o", CandidateModels: []string{"gpt-4o-mini"}, SampleSize: 20,
		Judge: &replaylab.JudgeConfig{Model: "judge-mock", MaxJudgeSamples: 3},
	})
	r := final.Results[0]
	if got := *r.JudgeWins + *r.JudgeTies + *r.JudgeLosses; got != 6 {
		t.Fatalf("judgements = %d, want 2 per judged replay for 3 replays", got)
	}
	if *r.JudgeCoverage != 15 {
		t.Fatalf("judge_coverage = %v, want 15 (3 of 20)", *r.JudgeCoverage)
	}
	if v := final.Recommendations[0].Verdict; v == replaylab.VerdictSafeToSwitch {
		t.Fatalf("verdict = %s; low coverage from the limit must cap it", v)
	}
	if len(r.JudgeSamples) != 3 {
		t.Fatalf("judge_samples = %d, want 3", len(r.JudgeSamples))
	}
}

func TestJudgeSamplesAreBoundedAndAuditable(t *testing.T) {
	up := judgeUpstream(t, map[string]string{"gpt-4o-mini": "Indivisible steps finish with no preemption."}, nil)
	lab := newLab(t, up.URL)
	seed(t, lab.Storage(), 20, "gpt-4o")

	final := runExperiment(t, lab, replaylab.CreateExperimentRequest{
		OriginalModel: "gpt-4o", CandidateModels: []string{"gpt-4o-mini"}, SampleSize: 20,
		Judge: &replaylab.JudgeConfig{Model: "judge-mock"},
	})
	samples := final.Results[0].JudgeSamples
	if len(samples) != replaylab.MaxStoredJudgements {
		t.Fatalf("judge_samples = %d, want %d", len(samples), replaylab.MaxStoredJudgements)
	}
	s := samples[0]
	if s.CaptureID == "" || !strings.Contains(s.PromptExcerpt, "atomic") || s.BaselineExcerpt != baselineAnswer || !strings.Contains(s.CandidateExcerpt, "Indivisible") {
		t.Fatalf("sample is missing its excerpts: %+v", s)
	}
	if len(s.Orders) != 2 || s.Orders[0].BaselinePosition != "A" || s.Orders[1].BaselinePosition != "B" {
		t.Fatalf("sample should hold both orders: %+v", s.Orders)
	}
	for _, o := range s.Orders {
		if o.Winner == "" || o.Reason == "" || o.CandidateOutcome == "" {
			t.Fatalf("order is missing its winner or reason: %+v", o)
		}
	}
}

func TestJudgeRubricDefaultsAndReachesTheJudge(t *testing.T) {
	const rubric = "Factual accuracy matters more than tone."
	var mu sync.Mutex
	sawRubric := false
	up := judgeUpstream(t, map[string]string{"gpt-4o-mini": baselineAnswer}, func(w http.ResponseWriter, body []byte) bool {
		mu.Lock()
		sawRubric = sawRubric || bytes.Contains(body, []byte(rubric))
		mu.Unlock()
		return false
	})
	lab := newLab(t, up.URL)
	seed(t, lab.Storage(), 3, "gpt-4o")
	req := replaylab.CreateExperimentRequest{OriginalModel: "gpt-4o", CandidateModels: []string{"gpt-4o-mini"}, SampleSize: 3}

	req.Judge = &replaylab.JudgeConfig{Model: "judge-mock"}
	def := runExperiment(t, lab, req)
	if def.Judge.Rubric != replaylab.DefaultJudgeRubric {
		t.Fatalf("rubric = %q, want the default", def.Judge.Rubric)
	}

	req.Judge = &replaylab.JudgeConfig{Model: "judge-mock", Rubric: "  " + rubric + " "}
	custom := runExperiment(t, lab, req)
	if custom.Judge.Rubric != rubric {
		t.Fatalf("rubric = %q, want %q", custom.Judge.Rubric, rubric)
	}
	mu.Lock()
	defer mu.Unlock()
	if !sawRubric {
		t.Fatal("the operator's rubric never reached the judge")
	}
}

func TestCreateExperimentRejectsInvalidJudgeOptions(t *testing.T) {
	lab := newLab(t, "http://127.0.0.1:1")
	api := httptest.NewServer(lab.GetAPIHandler())
	defer api.Close()

	long := strings.Repeat("x", replaylab.MaxJudgeRubricLength+1)
	for name, body := range map[string]string{
		"rubric too long":           `{"candidate_models":["gpt-4o-mini"],"judge":{"model":"judge-mock","rubric":"` + long + `"}}`,
		"negative max samples":      `{"candidate_models":["gpt-4o-mini"],"judge":{"model":"judge-mock","max_judge_samples":-1}}`,
		"unknown judge field":       `{"candidate_models":["gpt-4o-mini"],"judge":{"model":"judge-mock","temperature":1}}`,
		"server-set field on judge": `{"candidate_models":["gpt-4o-mini"],"judge":{"model":"judge-mock","judge_price_estimated":true}}`,
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

func TestProgressEventsCarryRunningJudgeScore(t *testing.T) {
	up := judgeUpstream(t, map[string]string{"gpt-4o-mini": baselineAnswer}, nil)
	lab := newLab(t, up.URL)
	seed(t, lab.Storage(), 10, "gpt-4o")

	collect := func(judge *replaylab.JudgeConfig) []replaylab.ReplayEvent {
		t.Helper()
		exp, err := lab.Engine().StartExperiment(replaylab.CreateExperimentRequest{
			OriginalModel: "gpt-4o", CandidateModels: []string{"gpt-4o-mini"}, SampleSize: 10, Judge: judge,
		})
		if err != nil {
			t.Fatal(err)
		}
		events, unsubscribe := lab.Engine().SubscribeEvents(exp.ID)
		defer unsubscribe()
		var got []replaylab.ReplayEvent
		timeout := time.After(10 * time.Second)
		for {
			select {
			case ev, ok := <-events:
				if !ok {
					return got
				}
				got = append(got, ev)
				if ev.Final {
					return got
				}
			case <-timeout:
				t.Fatal("no final event")
			}
		}
	}

	sawScore := false
	for _, ev := range collect(&replaylab.JudgeConfig{Model: "judge-mock"}) {
		if ev.CurrentJudgeScore != nil {
			sawScore = true
			if *ev.CurrentJudgeScore != 100 {
				t.Fatalf("current_judge_score = %v, want 100", *ev.CurrentJudgeScore)
			}
		}
	}
	if !sawScore {
		t.Fatal("no progress event carried current_judge_score")
	}
	for _, ev := range collect(nil) {
		raw, _ := json.Marshal(ev)
		if strings.Contains(string(raw), "judge") {
			t.Fatalf("event without a judge exposes judge fields: %s", raw)
		}
	}
}

func TestReportShowsJudgeResults(t *testing.T) {
	up := judgeUpstream(t, map[string]string{"gpt-4o-mini": baselineAnswer}, nil)
	lab := newLab(t, up.URL)
	seed(t, lab.Storage(), 5, "gpt-4o")
	req := replaylab.CreateExperimentRequest{OriginalModel: "gpt-4o", CandidateModels: []string{"gpt-4o-mini"}, SampleSize: 5}

	plain := runExperiment(t, lab, req).SummaryMarkdown
	if strings.Contains(plain, "Judge") {
		t.Fatalf("report without a judge mentions the judge:\n%s", plain)
	}

	req.Judge = &replaylab.JudgeConfig{Model: "judge-mock", Rubric: "Accuracy over tone."}
	final := runExperiment(t, lab, req)
	if !final.JudgePriceEstimated {
		t.Fatal("judge-mock has no known price; judge cost must be flagged as estimated")
	}
	report := final.SummaryMarkdown
	for _, want := range []string{
		"**Judge model:** `judge-mock`", "**Judge rubric:** Accuracy over tone.", "fallback rate",
		"Judge score (0-100)", "Judge W/T/L", "Judge disagreement", "Judge coverage", "Similarity (0-100)",
		"| **100.0** | 0 / 10 / 0 | 0% | 100% |",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("report is missing %q:\n%s", want, report)
		}
	}
}
