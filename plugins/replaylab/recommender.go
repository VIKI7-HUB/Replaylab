package replaylab

import (
	"fmt"
	"math"
	"strings"
)

// Verdict thresholds. Kept together so the decision rules are auditable.
const (
	QualitySafeThreshold    = 85.0 // >= : quality preserved
	QualityCautionThreshold = 70.0 // >= : marginal; below: degraded
	MaxErrorRatePercent     = 5.0  // above: do not switch
	MaxLatencyRegression    = 40.0 // % slower than baseline before caution
	MinJSONValidityPercent  = 95.0 // below: JSON contract at risk
	MinConfidentSample      = 20   // fewer successful replays: caution at best
)

// GenerateRecommendation turns a candidate's measured results into a verdict
// and projects monthly savings over monthlyVolume requests. A zero volume
// leaves the projection at zero rather than inventing a traffic number.
func GenerateRecommendation(origModel, candModel string, res *ExperimentResult, monthlyVolume int64) SwitchRecommendation {
	verdict := VerdictSafeToSwitch
	var reasons []string

	downgrade := func(to string) {
		if to == VerdictDoNotSwitch || verdict == VerdictSafeToSwitch {
			verdict = to
		}
	}

	if res.SuccessfulRequests == 0 {
		verdict = VerdictDoNotSwitch
		reasons = append(reasons, "No successful replays; the candidate could not be evaluated")
	}

	if res.ErrorRate > MaxErrorRatePercent {
		downgrade(VerdictDoNotSwitch)
		reasons = append(reasons, fmt.Sprintf("High error rate (%.1f%%)", res.ErrorRate))
	}

	if res.SuccessfulRequests > 0 {
		switch {
		case res.AvgQualityScore < QualityCautionThreshold:
			downgrade(VerdictDoNotSwitch)
			reasons = append(reasons, fmt.Sprintf("Quality degraded (%.1f/100)", res.AvgQualityScore))
		case res.AvgQualityScore < QualitySafeThreshold:
			downgrade(VerdictSwitchWithCaution)
			reasons = append(reasons, fmt.Sprintf("Quality acceptable but below baseline (%.1f/100)", res.AvgQualityScore))
		default:
			reasons = append(reasons, fmt.Sprintf("High quality retention (%.1f/100)", res.AvgQualityScore))
		}
	}

	if res.JSONSamples > 0 && res.JSONValidityRate < MinJSONValidityPercent {
		downgrade(VerdictSwitchWithCaution)
		if res.JSONValidityRate < QualityCautionThreshold {
			downgrade(VerdictDoNotSwitch)
		}
		reasons = append(reasons, fmt.Sprintf("JSON outputs valid in only %.1f%% of %d JSON prompts", res.JSONValidityRate, res.JSONSamples))
	}

	switch {
	case res.LatencyDeltaPercent > MaxLatencyRegression:
		downgrade(VerdictSwitchWithCaution)
		reasons = append(reasons, fmt.Sprintf("Latency increased by %.1f%% (p95: %.0fms)", res.LatencyDeltaPercent, res.P95LatencyMs))
	case res.LatencyDeltaPercent < 0:
		reasons = append(reasons, fmt.Sprintf("Latency improved by %.1f%%", math.Abs(res.LatencyDeltaPercent)))
	}

	if res.SuccessfulRequests > 0 {
		if res.CostSavingsPercent < 0 {
			downgrade(VerdictSwitchWithCaution)
			reasons = append(reasons, fmt.Sprintf("Candidate is %.1f%% more expensive", math.Abs(res.CostSavingsPercent)))
		} else {
			reasons = append(reasons, fmt.Sprintf("Cost savings of %.1f%%", res.CostSavingsPercent))
		}
	}

	if res.SuccessfulRequests > 0 && res.SuccessfulRequests < MinConfidentSample {
		downgrade(VerdictSwitchWithCaution)
		reasons = append(reasons, fmt.Sprintf("Small sample (%d replays); rerun with more traffic before switching", res.SuccessfulRequests))
	}

	// Savings are projected per successful replay: failed replays carry no cost signal.
	var projected float64
	if res.SuccessfulRequests > 0 && monthlyVolume > 0 {
		perRequest := res.CostSavingsDollars / float64(res.SuccessfulRequests)
		projected = math.Max(0, perRequest*float64(monthlyVolume))
	}

	res.Recommendation = verdict
	res.Reasoning = strings.Join(reasons, "; ")
	res.ProjectedMonthlySavings = math.Round(projected*100) / 100

	return SwitchRecommendation{
		OriginalModel:           origModel,
		CandidateModel:          candModel,
		Verdict:                 verdict,
		Confidence:              recommendationConfidence(res),
		ProjectedMonthlySavings: res.ProjectedMonthlySavings,
		Reasoning:               res.Reasoning,
	}
}

// recommendationConfidence reflects how much evidence backs the verdict: it
// grows with the number of successful replays (n/(n+20)) and shrinks with
// the error rate. It is a sample-size signal, not a statistical p-value.
func recommendationConfidence(res *ExperimentResult) float64 {
	n := float64(res.SuccessfulRequests)
	if n == 0 {
		return 0
	}
	c := n / (n + 20) * (1 - res.ErrorRate/100)
	return math.Round(c*100) / 100
}

func verdictLabel(v string) string {
	switch v {
	case VerdictSafeToSwitch:
		return "**SAFE TO SWITCH**"
	case VerdictSwitchWithCaution:
		return "*SWITCH WITH CAUTION*"
	case VerdictDoNotSwitch:
		return "DO NOT SWITCH"
	}
	return v
}

// FormatMarkdownReport renders a shareable experiment summary.
func FormatMarkdownReport(exp *Experiment) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "# Bifrost Replay Lab: Experiment Report (%s)\n\n", exp.Name)
	fmt.Fprintf(&sb, "- **Experiment ID:** `%s`\n", exp.ID)
	fmt.Fprintf(&sb, "- **Status:** `%s`\n", exp.Status)
	if exp.StopReason != "" {
		fmt.Fprintf(&sb, "- **Stop reason:** %s\n", exp.StopReason)
	}
	fmt.Fprintf(&sb, "- **Baseline Model:** `%s`\n", exp.OriginalModel)
	fmt.Fprintf(&sb, "- **Replays Run:** %d / %d\n", exp.CompletedReplays, exp.TotalReplays)
	fmt.Fprintf(&sb, "- **Budget Spent:** $%.4f / $%.2f cap\n", exp.SpentDollars, exp.BudgetCapDollars)
	if exp.MonthlyRequestVolume > 0 {
		fmt.Fprintf(&sb, "- **Monthly volume used for projections:** %d requests\n\n", exp.MonthlyRequestVolume)
	} else {
		sb.WriteString("- **Monthly volume used for projections:** unknown (no projection)\n\n")
	}

	if len(exp.Results) == 0 {
		sb.WriteString("No candidate produced results.\n")
		return sb.String()
	}

	sb.WriteString("## Candidate Comparison\n\n")
	sb.WriteString("| Candidate | Quality (0-100) | JSON valid | p50 / p95 latency (vs baseline) | Errors | Cost savings | Verdict | Projected monthly savings |\n")
	sb.WriteString("| :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- |\n")
	for _, r := range exp.Results {
		jsonCol := "n/a"
		if r.JSONSamples > 0 {
			jsonCol = fmt.Sprintf("%.0f%% of %d", r.JSONValidityRate, r.JSONSamples)
		}
		fmt.Fprintf(&sb,
			"| `%s` | **%.1f** | %s | %.0fms / %.0fms (%+.0f%%) | %.1f%% | **%.1f%%** ($%.4f) | %s | **$%.2f/mo** |\n",
			r.CandidateModel, r.AvgQualityScore, jsonCol,
			r.P50LatencyMs, r.P95LatencyMs, r.LatencyDeltaPercent,
			r.ErrorRate, r.CostSavingsPercent, r.CostSavingsDollars,
			verdictLabel(r.Recommendation), r.ProjectedMonthlySavings,
		)
	}

	sb.WriteString("\n## Decision Guidance\n\n")
	for _, rec := range exp.Recommendations {
		fmt.Fprintf(&sb, "### `%s` -> `%s`\n", rec.OriginalModel, rec.CandidateModel)
		fmt.Fprintf(&sb, "- **Verdict:** `%s` (confidence %.2f)\n", rec.Verdict, rec.Confidence)
		fmt.Fprintf(&sb, "- **Projected monthly savings:** $%.2f\n", rec.ProjectedMonthlySavings)
		fmt.Fprintf(&sb, "- **Rationale:** %s\n\n", rec.Reasoning)
	}

	sb.WriteString("> Quality is a lexical/structural similarity score against the baseline response, not a semantic judgement. Review sample outputs before switching production traffic.\n")
	return sb.String()
}
