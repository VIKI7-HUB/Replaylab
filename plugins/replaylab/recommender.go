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

	// Used instead of the quality thresholds when a candidate has a judge score.
	JudgeSafeThreshold          = 80.0 // >= : judge rates the candidate as good as the baseline
	JudgeCautionThreshold       = 60.0 // >= : marginal; below: worse than baseline
	MinJudgeCoveragePercent     = 80.0 // fewer replays judged: caution at best
	MaxJudgeDisagreementPercent = 30.0 // above: judge too inconsistent to trust fully
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

	if res.SuccessfulRequests > 0 && res.JudgeScore != nil {
		score := *res.JudgeScore
		switch {
		case score < JudgeCautionThreshold:
			downgrade(VerdictDoNotSwitch)
			reasons = append(reasons, fmt.Sprintf("Judge rates the candidate worse than the baseline (judge score %.1f/100)", score))
		case score < JudgeSafeThreshold:
			downgrade(VerdictSwitchWithCaution)
			reasons = append(reasons, fmt.Sprintf("Judge rates the candidate slightly below the baseline (judge score %.1f/100)", score))
		default:
			reasons = append(reasons, fmt.Sprintf("Judge rates the candidate as good as the baseline (judge score %.1f/100)", score))
		}
	} else if res.SuccessfulRequests > 0 {
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

	if res.SuccessfulRequests > 0 && res.JudgeCoverage != nil && *res.JudgeCoverage < MinJudgeCoveragePercent {
		downgrade(VerdictSwitchWithCaution)
		reasons = append(reasons, fmt.Sprintf("Judge covered only %.0f%% of successful replays; the rest fell back to similarity scoring", *res.JudgeCoverage))
	}
	if res.JudgeDisagreementRate != nil && *res.JudgeDisagreementRate > MaxJudgeDisagreementPercent {
		downgrade(VerdictSwitchWithCaution)
		reasons = append(reasons, fmt.Sprintf("Judge contradicted itself across response orders in %.0f%% of replays", *res.JudgeDisagreementRate))
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
	if exp.Judge != nil {
		fmt.Fprintf(&sb, "- **Judge model:** `%s`", exp.Judge.Model)
		if exp.JudgeSpentDollars != nil {
			fmt.Fprintf(&sb, " (spent $%.4f of the budget)", *exp.JudgeSpentDollars)
		}
		sb.WriteString("\n")
		fmt.Fprintf(&sb, "- **Judge rubric:** %s\n", oneLine(exp.Judge.Rubric))
		if exp.Judge.MaxJudgeSamples > 0 {
			fmt.Fprintf(&sb, "- **Judged replays per candidate:** at most %d\n", exp.Judge.MaxJudgeSamples)
		}
		if exp.JudgePriceEstimated {
			fmt.Fprintf(&sb, "- **Warning:** `%s` has no known price; judge cost is estimated at the fallback rate ($%.2f in / $%.2f out per 1M tokens)\n",
				exp.Judge.Model, fallbackPricing.InputPer1M, fallbackPricing.OutputPer1M)
		}
	}
	if exp.MonthlyRequestVolume > 0 {
		fmt.Fprintf(&sb, "- **Monthly volume used for projections:** %d requests\n\n", exp.MonthlyRequestVolume)
	} else {
		sb.WriteString("- **Monthly volume used for projections:** unknown (no projection)\n\n")
	}

	if len(exp.Results) == 0 {
		sb.WriteString("No candidate produced results.\n")
		return sb.String()
	}

	judged := false
	for _, r := range exp.Results {
		if r.JudgeCoverage != nil {
			judged = true
		}
	}

	sb.WriteString("## Candidate Comparison\n\n")
	if judged {
		sb.WriteString("| Candidate | Judge score (0-100) | Judge W/T/L | Judge disagreement | Judge coverage | Similarity (0-100) | JSON valid | p50 / p95 latency (vs baseline) | Errors | Cost savings | Verdict | Projected monthly savings |\n")
		sb.WriteString("| :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- |\n")
	} else {
		sb.WriteString("| Candidate | Quality (0-100) | JSON valid | p50 / p95 latency (vs baseline) | Errors | Cost savings | Verdict | Projected monthly savings |\n")
		sb.WriteString("| :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- |\n")
	}
	for _, r := range exp.Results {
		jsonCol := "n/a"
		if r.JSONSamples > 0 {
			jsonCol = fmt.Sprintf("%.0f%% of %d", r.JSONValidityRate, r.JSONSamples)
		}
		if judged {
			fmt.Fprintf(&sb, "| `%s` | %s | %s | %s | %s ", r.CandidateModel,
				fmtOpt(r.JudgeScore, "**%.1f**"), fmtWTL(r), fmtOpt(r.JudgeDisagreementRate, "%.0f%%"), fmtOpt(r.JudgeCoverage, "%.0f%%"))
		} else {
			fmt.Fprintf(&sb, "| `%s` ", r.CandidateModel)
		}
		fmt.Fprintf(&sb,
			"| **%.1f** | %s | %.0fms / %.0fms (%+.0f%%) | %.1f%% | **%.1f%%** ($%.4f) | %s | **$%.2f/mo** |\n",
			r.AvgQualityScore, jsonCol,
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

	if judged {
		sb.WriteString("> Verdicts use the judge score: each replay is judged against the baseline response twice, with the responses in swapped positions. Similarity is the lexical/structural score, kept to spot formatting drift. Review the judge samples before switching production traffic.\n")
	} else {
		sb.WriteString("> Quality is a lexical/structural similarity score against the baseline response, not a semantic judgement. Review sample outputs before switching production traffic.\n")
	}
	return sb.String()
}

func fmtOpt(v *float64, format string) string {
	if v == nil {
		return "n/a"
	}
	return fmt.Sprintf(format, *v)
}

func fmtWTL(r ExperimentResult) string {
	if r.JudgeWins == nil || r.JudgeTies == nil || r.JudgeLosses == nil {
		return "n/a"
	}
	return fmt.Sprintf("%d / %d / %d", *r.JudgeWins, *r.JudgeTies, *r.JudgeLosses)
}

// oneLine keeps operator text from breaking the Markdown list it is rendered in.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
