package replaylab

import (
	"fmt"
	"math"
	"strings"
)

// GenerateRecommendation determines the verdict and extrapolates monthly savings.
func GenerateRecommendation(origModel, candModel string, res *ExperimentResult, monthlyTrafficEstimate int) SwitchRecommendation {
	if monthlyTrafficEstimate <= 0 {
		monthlyTrafficEstimate = 1000000 // 1 Million requests/month default baseline
	}

	verdict := VerdictSafeToSwitch
	var reasons []string

	// Check error rate
	if res.ErrorRate > 5.0 {
		verdict = VerdictDoNotSwitch
		reasons = append(reasons, fmt.Sprintf("High error rate (%.1f%%)", res.ErrorRate))
	}

	// Check quality score
	if res.AvgQualityScore < 70.0 {
		verdict = VerdictDoNotSwitch
		reasons = append(reasons, fmt.Sprintf("Quality score is degraded (%.1f/100)", res.AvgQualityScore))
	} else if res.AvgQualityScore < 85.0 {
		if verdict != VerdictDoNotSwitch {
			verdict = VerdictSwitchWithCaution
		}
		reasons = append(reasons, fmt.Sprintf("Quality score is acceptable but lower than baseline (%.1f/100)", res.AvgQualityScore))
	} else {
		reasons = append(reasons, fmt.Sprintf("High quality retention (%.1f/100)", res.AvgQualityScore))
	}

	// Check latency
	if res.LatencyDeltaPercent > 40.0 {
		if verdict == VerdictSafeToSwitch {
			verdict = VerdictSwitchWithCaution
		}
		reasons = append(reasons, fmt.Sprintf("Latency increased by +%.1f%% (p95: %.0fms)", res.LatencyDeltaPercent, res.P95LatencyMs))
	} else if res.LatencyDeltaPercent < 0 {
		reasons = append(reasons, fmt.Sprintf("Latency improved by %.1f%% (faster response)", math.Abs(res.LatencyDeltaPercent)))
	}

	// Check cost savings
	if res.CostSavingsPercent < 0 {
		if verdict != VerdictDoNotSwitch {
			verdict = VerdictSwitchWithCaution
		}
		reasons = append(reasons, fmt.Sprintf("Candidate model is more expensive by +%.1f%%", math.Abs(res.CostSavingsPercent)))
	} else {
		reasons = append(reasons, fmt.Sprintf("Cost savings of %.1f%%", res.CostSavingsPercent))
	}

	// Extrapolate monthly savings
	perReqSavings := 0.0
	if res.TotalRequests > 0 {
		perReqSavings = res.CostSavingsDollars / float64(res.TotalRequests)
	}
	projectedMonthlySavings := perReqSavings * float64(monthlyTrafficEstimate)
	if projectedMonthlySavings < 0 {
		projectedMonthlySavings = 0
	}

	res.Recommendation = verdict
	res.Reasoning = strings.Join(reasons, "; ")
	res.ProjectedMonthlySavings = math.Round(projectedMonthlySavings*100) / 100

	return SwitchRecommendation{
		OriginalModel:           origModel,
		CandidateModel:          candModel,
		Verdict:                 verdict,
		Confidence:              math.Min(0.98, res.AvgQualityScore/100.0),
		ProjectedMonthlySavings: res.ProjectedMonthlySavings,
		Reasoning:               res.Reasoning,
	}
}

// FormatMarkdownReport builds an executive comparison summary table and markdown report.
func FormatMarkdownReport(exp *Experiment) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# Bifrost Replay Lab: Experiment Report (%s)\n\n", exp.Name))
	sb.WriteString(fmt.Sprintf("- **Experiment ID:** `%s`\n", exp.ID))
	sb.WriteString(fmt.Sprintf("- **Baseline Model:** `%s`\n", exp.OriginalModel))
	sb.WriteString(fmt.Sprintf("- **Total Replays Run:** %d / %d\n", exp.CompletedReplays, exp.TotalReplays))
	sb.WriteString(fmt.Sprintf("- **Total Budget Spent:** $%.4f / $%.2f cap\n\n", exp.SpentDollars, exp.BudgetCapDollars))

	sb.WriteString("## Candidate Model Comparison & Recommendations\n\n")
	sb.WriteString("| Candidate Model | Quality (0-100) | p50 / p95 Latency | Cost Savings | Verdict | Projected Monthly Savings |\n")
	sb.WriteString("| :--- | :--- | :--- | :--- | :--- | :--- |\n")

	for _, r := range exp.Results {
		verdictBadge := r.Recommendation
		switch r.Recommendation {
		case VerdictSafeToSwitch:
			verdictBadge = "**SAFE TO SWITCH**"
		case VerdictSwitchWithCaution:
			verdictBadge = "*SWITCH WITH CAUTION*"
		case VerdictDoNotSwitch:
			verdictBadge = "DO NOT SWITCH"
		}

		sb.WriteString(fmt.Sprintf(
			"| `%s` | **%.1f** | %.0fms / %.0fms (%.0f%%) | **%.1f%%** ($%.4f) | %s | **$%.2f/mo** |\n",
			r.CandidateModel,
			r.AvgQualityScore,
			r.P50LatencyMs,
			r.P95LatencyMs,
			r.LatencyDeltaPercent,
			r.CostSavingsPercent,
			r.CostSavingsDollars,
			verdictBadge,
			r.ProjectedMonthlySavings,
		))
	}

	sb.WriteString("\n## Strategic Decision Guidance\n\n")
	for _, rec := range exp.Recommendations {
		sb.WriteString(fmt.Sprintf("### `%s` -> `%s`\n", rec.OriginalModel, rec.CandidateModel))
		sb.WriteString(fmt.Sprintf("- **Verdict:** `%s`\n", rec.Verdict))
		sb.WriteString(fmt.Sprintf("- **Projected Monthly ROI:** $%.2f / month\n", rec.ProjectedMonthlySavings))
		sb.WriteString(fmt.Sprintf("- **Operator Rationale:** %s\n\n", rec.Reasoning))
	}

	return sb.String()
}
