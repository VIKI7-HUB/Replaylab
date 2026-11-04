package replaylab

import (
	"encoding/json"
	"math"
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

// ComputeQualityScore evaluates candidate response text against the original response.
// Returns a 0-100 score, JSON validity boolean, and explanation notes.
func (qs *QualityScorer) ComputeQualityScore(origText, candText string) (float64, bool, []string) {
	origTrimmed := strings.TrimSpace(origText)
	candTrimmed := strings.TrimSpace(candText)

	if origTrimmed == "" && candTrimmed == "" {
		return 100.0, true, []string{"Both empty"}
	}
	if origTrimmed != "" && candTrimmed == "" {
		return 0.0, false, []string{"Candidate returned empty response"}
	}

	var notes []string

	// 1. Exact Match Check (100%)
	if origTrimmed == candTrimmed {
		return 100.0, true, []string{"Exact match"}
	}

	// 2. JSON Validity and Key Match Check
	isOrigJSON, origKeys := extractJSONKeys(origTrimmed)
	isCandJSON, candKeys := extractJSONKeys(candTrimmed)

	var jsonScore float64 = 100.0
	var jsonValid bool = true

	if isOrigJSON {
		if !isCandJSON {
			jsonScore = 20.0
			jsonValid = false
			notes = append(notes, "Original was valid JSON but candidate output failed JSON parsing")
		} else {
			// Compare JSON keys overlap
			keyOverlap := computeKeyOverlap(origKeys, candKeys)
			jsonScore = keyOverlap * 100.0
			notes = append(notes, "JSON schema keys matched")
		}
	}

	// 3. Length Ratio Metric (0 - 100)
	lenOrig := float64(len(origTrimmed))
	lenCand := float64(len(candTrimmed))
	ratio := math.Min(lenOrig, lenCand) / math.Max(lenOrig, lenCand)
	lengthScore := ratio * 100.0

	// 4. Token / Jaccard Lexical Similarity (0 - 100)
	jaccardScore := computeJaccardSimilarity(origTrimmed, candTrimmed) * 100.0

	// Aggregate Weighted Quality Score
	var finalQuality float64
	if isOrigJSON {
		finalQuality = (0.50 * jsonScore) + (0.30 * jaccardScore) + (0.20 * lengthScore)
	} else {
		finalQuality = (0.65 * jaccardScore) + (0.35 * lengthScore)
	}

	finalQuality = math.Round(math.Max(0.0, math.Min(100.0, finalQuality))*10) / 10
	return finalQuality, jsonValid, notes
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

// ComputePercentiles calculates p50 and p95 latency values in milliseconds.
func ComputePercentiles(latencies []float64) (float64, float64) {
	if len(latencies) == 0 {
		return 0, 0
	}
	sorted := make([]float64, len(latencies))
	copy(sorted, latencies)
	sort.Float64s(sorted)

	p50Idx := int(float64(len(sorted)) * 0.50)
	p95Idx := int(float64(len(sorted)) * 0.95)

	if p50Idx >= len(sorted) {
		p50Idx = len(sorted) - 1
	}
	if p95Idx >= len(sorted) {
		p95Idx = len(sorted) - 1
	}

	return sorted[p50Idx], sorted[p95Idx]
}
