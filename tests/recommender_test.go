package tests

import (
	"testing"

	"github.com/maximhq/bifrost/plugins/replaylab/plugins/replaylab"
)

func TestRecommendationSafeToSwitch(t *testing.T) {
	res := &replaylab.ExperimentResult{
		TotalRequests:        100,
		SuccessfulRequests:   100,
		OriginalCostDollars:  10.0,
		CandidateCostDollars: 1.0,
		CostSavingsDollars:   9.0,
		CostSavingsPercent:   90.0,
		AvgQualityScore:      95.0,
		LatencyDeltaPercent:  -20.0,
		ErrorRate:            0.0,
	}

	rec := replaylab.GenerateRecommendation("gpt-4o", "gpt-4o-mini", res, 1000000)
	if rec.Verdict != replaylab.VerdictSafeToSwitch {
		t.Errorf("Expected SAFE_TO_SWITCH, got %s", rec.Verdict)
	}
	if rec.ProjectedMonthlySavings <= 0 {
		t.Errorf("Expected positive monthly savings, got %.2f", rec.ProjectedMonthlySavings)
	}
}

func TestRecommendationDoNotSwitchOnError(t *testing.T) {
	res := &replaylab.ExperimentResult{
		TotalRequests:        100,
		SuccessfulRequests:   80,
		FailedRequests:       20,
		OriginalCostDollars:  10.0,
		CandidateCostDollars: 1.0,
		CostSavingsDollars:   9.0,
		CostSavingsPercent:   90.0,
		AvgQualityScore:      92.0,
		ErrorRate:            20.0, // High error rate
	}

	rec := replaylab.GenerateRecommendation("gpt-4o", "local-broken", res, 1000000)
	if rec.Verdict != replaylab.VerdictDoNotSwitch {
		t.Errorf("Expected DO_NOT_SWITCH due to errors, got %s", rec.Verdict)
	}
}

func TestRecommendationCautionOnDegradedQuality(t *testing.T) {
	res := &replaylab.ExperimentResult{
		TotalRequests:        100,
		SuccessfulRequests:   100,
		OriginalCostDollars:  10.0,
		CandidateCostDollars: 2.0,
		CostSavingsDollars:   8.0,
		CostSavingsPercent:   80.0,
		AvgQualityScore:      78.0, // Marginal quality
		LatencyDeltaPercent:  10.0,
		ErrorRate:            0.0,
	}

	rec := replaylab.GenerateRecommendation("gpt-4o", "budget-model", res, 1000000)
	if rec.Verdict != replaylab.VerdictSwitchWithCaution {
		t.Errorf("Expected SWITCH_WITH_CAUTION, got %s", rec.Verdict)
	}
}
