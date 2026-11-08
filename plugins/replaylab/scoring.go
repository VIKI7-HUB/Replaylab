package replaylab

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
	"unicode"
)

// QualityScorer calculates cheap automatic quality similarity and validation metrics.
type QualityScorer struct{}

// NewQualityScorer creates a QualityScorer.
func NewQualityScorer() *QualityScorer {
	return &QualityScorer{}
}

// QualityResult is the outcome of comparing a candidate response to the
// baseline response for the same prompt.
type QualityResult struct {
	Score float64 // 0-100
	// BaselineJSON is true when the baseline response was JSON, i.e. when
	// CandidateJSONValid is meaningful.
	BaselineJSON       bool
	CandidateJSONValid bool
	Notes              []string
}

// Score compares a candidate response against the baseline response.
//
// JSON baselines weight structural fidelity (key overlap) highest, because a
// cheaper model that breaks a JSON contract breaks the caller. Free-text
// baselines use lexical (Jaccard) similarity plus a length-ratio term.
func (qs *QualityScorer) Score(origText, candText string) QualityResult {
	origTrimmed := stripCodeFence(strings.TrimSpace(origText))
	candTrimmed := stripCodeFence(strings.TrimSpace(candText))

	isOrigJSON, origKeys := extractJSONKeys(origTrimmed)
	isCandJSON, candKeys := extractJSONKeys(candTrimmed)
	res := QualityResult{BaselineJSON: isOrigJSON, CandidateJSONValid: !isOrigJSON || isCandJSON}

	switch {
	case origTrimmed == "" && candTrimmed == "":
		res.Score, res.CandidateJSONValid = 100, true
		res.Notes = []string{"Both empty"}
		return res
	case candTrimmed == "":
		res.Score, res.CandidateJSONValid = 0, false
		res.Notes = []string{"Candidate returned empty response"}
		return res
	case origTrimmed == candTrimmed:
		res.Score, res.CandidateJSONValid = 100, true
		res.Notes = []string{"Exact match"}
		return res
	}

	jsonScore := 100.0
	if isOrigJSON {
		if !isCandJSON {
			jsonScore = 20.0
			res.Notes = append(res.Notes, "Baseline was JSON but candidate output failed JSON parsing")
		} else {
			overlap := computeKeyOverlap(origKeys, candKeys)
			jsonScore = overlap * 100.0
			res.Notes = append(res.Notes, fmt.Sprintf("JSON keys preserved: %.0f%%", jsonScore))
		}
	}

	lenOrig := float64(len(origTrimmed))
	lenCand := float64(len(candTrimmed))
	lengthScore := math.Min(lenOrig, lenCand) / math.Max(lenOrig, lenCand) * 100.0
	jaccardScore := computeJaccardSimilarity(origTrimmed, candTrimmed) * 100.0

	var final float64
	if isOrigJSON {
		final = (0.50 * jsonScore) + (0.30 * jaccardScore) + (0.20 * lengthScore)
	} else {
		final = (0.65 * jaccardScore) + (0.35 * lengthScore)
	}
	res.Score = math.Round(math.Max(0.0, math.Min(100.0, final))*10) / 10
	return res
}

// ComputeQualityScore is the tuple form of Score: (score, candidate JSON
// valid, notes). For non-JSON baselines the validity flag is always true.
func (qs *QualityScorer) ComputeQualityScore(origText, candText string) (float64, bool, []string) {
	r := qs.Score(origText, candText)
	return r.Score, r.CandidateJSONValid, r.Notes
}

// stripCodeFence removes a surrounding ```json ... ``` fence, which models
// often wrap around otherwise valid JSON.
func stripCodeFence(s string) string {
	if !strings.HasPrefix(s, "```") || !strings.HasSuffix(s, "```") || len(s) < 6 {
		return s
	}
	inner := strings.TrimSuffix(strings.TrimPrefix(s, "```"), "```")
	if nl := strings.IndexByte(inner, '\n'); nl >= 0 && !strings.ContainsAny(inner[:nl], "{[") {
		inner = inner[nl+1:] // drop the language tag line
	}
	return strings.TrimSpace(inner)
}

func extractJSONKeys(s string) (bool, map[string]struct{}) {
	var obj map[string]any
	if err := json.Unmarshal([]byte(s), &obj); err == nil {
		keys := make(map[string]struct{}, len(obj))
		for k := range obj {
			keys[k] = struct{}{}
		}
		return true, keys
	}
	// Try slice of objects
	var arr []map[string]any
	if err := json.Unmarshal([]byte(s), &arr); err == nil && len(arr) > 0 {
		keys := make(map[string]struct{}, len(arr[0]))
		for k := range arr[0] {
			keys[k] = struct{}{}
		}
		return true, keys
	}
	return false, nil
}

func computeKeyOverlap(orig, cand map[string]struct{}) float64 {
	if len(orig) == 0 {
		return 1.0
	}
	matched := 0
	for k := range orig {
		if _, ok := cand[k]; ok {
			matched++
		}
	}
	return float64(matched) / float64(len(orig))
}

func computeJaccardSimilarity(a, b string) float64 {
	wordsA := tokenize(a)
	wordsB := tokenize(b)

	setA := make(map[string]struct{}, len(wordsA))
	for _, w := range wordsA {
		setA[w] = struct{}{}
	}

	setB := make(map[string]struct{}, len(wordsB))
	for _, w := range wordsB {
		setB[w] = struct{}{}
	}

	if len(setA) == 0 && len(setB) == 0 {
		return 1.0
	}

	intersection := 0
	for w := range setA {
		if _, ok := setB[w]; ok {
			intersection++
		}
	}

	union := len(setA)
	for w := range setB {
		if _, ok := setA[w]; !ok {
			union++
		}
	}

	if union == 0 {
		return 0.0
	}
	return float64(intersection) / float64(union)
}

func tokenize(s string) []string {
	fields := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
	return fields
}

// ComputePercentiles returns nearest-rank p50 and p95 latency values.
func ComputePercentiles(latencies []float64) (float64, float64) {
	if len(latencies) == 0 {
		return 0, 0
	}
	sorted := slices.Clone(latencies)
	sort.Float64s(sorted)
	return nearestRank(sorted, 0.50), nearestRank(sorted, 0.95)
}

func nearestRank(sorted []float64, p float64) float64 {
	idx := int(math.Ceil(p*float64(len(sorted)))) - 1
	idx = max(0, min(idx, len(sorted)-1))
	return sorted[idx]
}
