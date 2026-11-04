package tests

import (
	"testing"

	"github.com/maximhq/bifrost/plugins/replaylab/plugins/replaylab"
)

func TestExactMatchScoring(t *testing.T) {
	scorer := replaylab.NewQualityScorer()
	score, valid, notes := scorer.ComputeQualityScore("Hello world", "Hello world")
	if score != 100.0 {
		t.Errorf("Expected 100.0 for exact match, got %.1f", score)
	}
	if !valid {
		t.Errorf("Expected valid=true")
	}
	if len(notes) == 0 || notes[0] != "Exact match" {
		t.Errorf("Expected exact match note")
	}
}

func TestJSONValidityScoring(t *testing.T) {
	scorer := replaylab.NewQualityScorer()

	origJSON := `{"id": 1, "status": "active", "tier": "gold"}`
	candValidJSON := `{"id": 1, "status": "active", "tier": "gold", "meta": "extra"}`
	candBrokenJSON := `This is not json at all`

	scoreGood, validGood, _ := scorer.ComputeQualityScore(origJSON, candValidJSON)
	if scoreGood < 80.0 || !validGood {
		t.Errorf("Expected high quality score for valid JSON match, got %.1f, valid=%v", scoreGood, validGood)
	}

	scoreBad, validBad, _ := scorer.ComputeQualityScore(origJSON, candBrokenJSON)
	if validBad {
		t.Errorf("Expected valid=false for non-json candidate output")
	}
	if scoreBad > 40.0 {
		t.Errorf("Expected penalized score for broken JSON, got %.1f", scoreBad)
	}
}

func TestPercentilesCalculation(t *testing.T) {
	latencies := []float64{10, 20, 30, 40, 50, 60, 70, 80, 90, 100}
	p50, p95 := replaylab.ComputePercentiles(latencies)

	if p50 < 45 || p50 > 65 {
		t.Errorf("Expected p50 around 50-60, got %.1f", p50)
	}
	if p95 < 90 {
		t.Errorf("Expected p95 >= 90, got %.1f", p95)
	}
}
