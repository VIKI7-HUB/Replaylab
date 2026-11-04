package replaylab

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ReplayEvent represents a real-time progress update for SSE.
type ReplayEvent struct {
	ExperimentID    string  `json:"experiment_id"`
	CandidateModel  string  `json:"candidate_model"`
	CompletedCount  int     `json:"completed_count"`
	TotalCount      int     `json:"total_count"`
	ProgressPercent float64 `json:"progress_percent"`
	CurrentQuality  float64 `json:"current_quality"`
	SpentDollars    float64 `json:"spent_dollars"`
	Message         string  `json:"message"`
}

// ReplayEngine executes model comparative experiments with budget cap enforcement.
type ReplayEngine struct {
	storage       Storage
	scorer        *QualityScorer
	gatewayURL    string
	activeRuns    sync.Map // experimentID -> context.CancelFunc
	eventChannels sync.Map // experimentID -> []chan ReplayEvent
	eventMu       sync.Mutex
	httpClient    *http.Client
}

// NewReplayEngine creates and configures a ReplayEngine.
func NewReplayEngine(storage Storage, gatewayURL string) *ReplayEngine {
	if gatewayURL == "" {
		gatewayURL = "http://localhost:8080"
	}
	return &ReplayEngine{
		storage:    storage,
		scorer:     NewQualityScorer(),
		gatewayURL: gatewayURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// SubscribeEvents registers a listener channel for live progress updates.
func (re *ReplayEngine) SubscribeEvents(expID string) chan ReplayEvent {
	re.eventMu.Lock()
	defer re.eventMu.Unlock()

	ch := make(chan ReplayEvent, 100)
	val, _ := re.eventChannels.Load(expID)
	var list []chan ReplayEvent
	if val != nil {
		list = val.([]chan ReplayEvent)
	}
	list = append(list, ch)
	re.eventChannels.Store(expID, list)
	return ch
}

// broadcast emits an event to all active SSE subscribers.
func (re *ReplayEngine) broadcast(event ReplayEvent) {
	val, ok := re.eventChannels.Load(event.ExperimentID)
	if !ok || val == nil {
		return
	}
	list := val.([]chan ReplayEvent)
	for _, ch := range list {
		select {
		case ch <- event:
		default:
		}
	}
}

// CancelExperiment halts an ongoing run.
func (re *ReplayEngine) CancelExperiment(expID string) bool {
	val, ok := re.activeRuns.Load(expID)
	if !ok {
		return false
	}
	cancel := val.(context.CancelFunc)
	cancel()
	re.activeRuns.Delete(expID)
	return true
}

// EstimateDryRun calculates projected replays and cost before running.
func (re *ReplayEngine) EstimateDryRun(req CreateExperimentRequest) (*Experiment, error) {
	captures, err := re.storage.GetCaptures(CaptureFilter{
		Model: req.OriginalModel,
		Tag:   req.FilterTag,
		Limit: req.SampleSize,
	})
	if err != nil {
		return nil, err
	}
	if len(captures) == 0 {
		return nil, fmt.Errorf("no captured requests found matching model '%s'", req.OriginalModel)
	}

	totalReplays := len(captures) * len(req.CandidateModels)
	var estimatedCost float64

	for _, cand := range req.CandidateModels {
		for _, cap := range captures {
			estimatedCost += EstimateCost(cand, cap.PromptTokens, cap.CompletionTokens)
		}
	}

	exp := &Experiment{
		ID:               "exp-" + uuid.NewString()[:10],
		Name:             req.Name,
		Status:           StatusPending,
		OriginalModel:    req.OriginalModel,
		CandidateModels:  req.CandidateModels,
		FilterTag:        req.FilterTag,
		SampleSize:       len(captures),
		BudgetCapDollars: req.BudgetCapDollars,
		FixedTemperature: req.FixedTemperature,
		DryRun:           true,
		CreatedAt:        time.Now().UTC(),
		TotalReplays:     totalReplays,
		SpentDollars:     estimatedCost,
	}
	return exp, nil
}

// StartExperiment begins asynchronous replay of captured traffic.
func (re *ReplayEngine) StartExperiment(req CreateExperimentRequest) (*Experiment, error) {
	if req.DryRun {
		return re.EstimateDryRun(req)
	}

	captures, err := re.storage.GetCaptures(CaptureFilter{
		Model: req.OriginalModel,
		Tag:   req.FilterTag,
		Limit: req.SampleSize,
	})
	if err != nil {
		return nil, err
	}
	if len(captures) == 0 {
		return nil, fmt.Errorf("no captured requests found matching model '%s'", req.OriginalModel)
	}

	if req.BudgetCapDollars <= 0 {
		req.BudgetCapDollars = 50.0
	}

	origModel := req.OriginalModel
	if origModel == "" && len(captures) > 0 {
		origModel = captures[0].Model
	}

	exp := &Experiment{
		ID:               "exp-" + uuid.NewString()[:10],
		Name:             req.Name,
		Status:           StatusRunning,
		OriginalModel:    origModel,
		CandidateModels:  req.CandidateModels,
		FilterTag:        req.FilterTag,
		SampleSize:       len(captures),
		BudgetCapDollars: req.BudgetCapDollars,
		FixedTemperature: req.FixedTemperature,
		DryRun:           false,
		CreatedAt:        time.Now().UTC(),
		TotalReplays:     len(captures) * len(req.CandidateModels),
	}

	if err := re.storage.SaveExperiment(exp); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	re.activeRuns.Store(exp.ID, cancel)

	go re.runExperimentAsync(ctx, exp, captures)

	return exp, nil
}

func (re *ReplayEngine) runExperimentAsync(ctx context.Context, exp *Experiment, captures []*CaptureRecord) {
	defer re.activeRuns.Delete(exp.ID)

	var totalSpentDollars float64
	var totalSpentTokens int
	var completedReplays int

	var results []ExperimentResult
	var recommendations []SwitchRecommendation

	for _, candModel := range exp.CandidateModels {
		res := ExperimentResult{
			CandidateModel: candModel,
			TotalRequests:  len(captures),
		}

		var candidateLatencies []float64
		var originalLatencies []float64
		var qualityScores []float64
		var origCostTotal, candCostTotal float64
		validJSONCount := 0

		for _, cap := range captures {
			// Check context cancellation
			if ctx.Err() != nil {
				exp.Status = StatusCancelled
				_ = re.storage.SaveExperiment(exp)
				return
			}

			// Hard Budget Cap Check
			if totalSpentDollars >= exp.BudgetCapDollars {
				exp.Status = StatusCompleted
				exp.SummaryMarkdown = fmt.Sprintf("⚠️ Experiment stopped: Reached hard budget cap of $%.2f", exp.BudgetCapDollars)
				_ = re.storage.SaveExperiment(exp)
				return
			}

			// Execute replay against candidate model
			candResp, latencyMs, candCost, err := re.executeSingleReplay(ctx, candModel, cap, exp.FixedTemperature)
			completedReplays++
			exp.CompletedReplays = completedReplays

			if err != nil {
				res.FailedRequests++
			} else {
				res.SuccessfulRequests++
				candidateLatencies = append(candidateLatencies, float64(latencyMs))
				originalLatencies = append(originalLatencies, float64(cap.LatencyMs))

				totalSpentDollars += candCost
				candCostTotal += candCost
				origCostTotal += cap.CostEstimate
				totalSpentTokens += cap.PromptTokens + cap.CompletionTokens

				// Evaluate quality
				qScore, isJSONValid, _ := re.scorer.ComputeQualityScore(cap.ResponseJSON, candResp)
				qualityScores = append(qualityScores, qScore)
				if isJSONValid {
					validJSONCount++
				}
			}

			exp.SpentDollars = totalSpentDollars
			exp.SpentTokens = totalSpentTokens

			// Broadcast SSE progress
			currentAvgQuality := 0.0
			if len(qualityScores) > 0 {
				var sum float64
				for _, s := range qualityScores {
					sum += s
				}
				currentAvgQuality = sum / float64(len(qualityScores))
			}

			re.broadcast(ReplayEvent{
				ExperimentID:    exp.ID,
				CandidateModel:  candModel,
				CompletedCount:  completedReplays,
				TotalCount:      exp.TotalReplays,
				ProgressPercent: (float64(completedReplays) / float64(exp.TotalReplays)) * 100.0,
				CurrentQuality:  currentAvgQuality,
				SpentDollars:    totalSpentDollars,
				Message:         fmt.Sprintf("Replayed request %s -> %s", cap.ID, candModel),
			})

			// Polite pacing: 10ms between requests
			time.Sleep(10 * time.Millisecond)
		}

		// Calculate aggregates for candidate
		if res.SuccessfulRequests > 0 {
			res.OriginalCostDollars = origCostTotal
			res.CandidateCostDollars = candCostTotal
			res.CostSavingsDollars = origCostTotal - candCostTotal
			if origCostTotal > 0 {
				res.CostSavingsPercent = (res.CostSavingsDollars / origCostTotal) * 100.0
			}

			p50, p95 := ComputePercentiles(candidateLatencies)
			res.P50LatencyMs = p50
			res.P95LatencyMs = p95

			var sumOrigLat, sumCandLat float64
			for _, l := range originalLatencies {
				sumOrigLat += l
			}
			for _, l := range candidateLatencies {
				sumCandLat += l
			}
			res.OriginalAvgLatencyMs = sumOrigLat / float64(len(originalLatencies))
			res.CandidateAvgLatencyMs = sumCandLat / float64(len(candidateLatencies))
			if res.OriginalAvgLatencyMs > 0 {
				res.LatencyDeltaPercent = ((res.CandidateAvgLatencyMs - res.OriginalAvgLatencyMs) / res.OriginalAvgLatencyMs) * 100.0
			}

			var sumQ float64
			for _, q := range qualityScores {
				sumQ += q
			}
			res.AvgQualityScore = sumQ / float64(len(qualityScores))
			res.JSONValidityRate = (float64(validJSONCount) / float64(res.SuccessfulRequests)) * 100.0
		}
		res.ErrorRate = (float64(res.FailedRequests) / float64(res.TotalRequests)) * 100.0

		// Generate switch recommendation
		rec := GenerateRecommendation(exp.OriginalModel, candModel, &res, 1000000)
		results = append(results, res)
		recommendations = append(recommendations, rec)
	}

	now := time.Now().UTC()
	exp.CompletedAt = &now
	exp.Status = StatusCompleted
	exp.Results = results
	exp.Recommendations = recommendations
	exp.SummaryMarkdown = FormatMarkdownReport(exp)

	_ = re.storage.SaveExperiment(exp)

	// Emit final completion event
	re.broadcast(ReplayEvent{
		ExperimentID:    exp.ID,
		CompletedCount:  exp.TotalReplays,
		TotalCount:      exp.TotalReplays,
		ProgressPercent: 100.0,
		SpentDollars:    exp.SpentDollars,
		Message:         "Experiment completed successfully!",
	})
}

func (re *ReplayEngine) executeSingleReplay(ctx context.Context, targetModel string, cap *CaptureRecord, fixedTemp *float64) (string, int64, float64, error) {
	// Reconstruct OpenAI-compatible chat completion payload
	payloadMap := map[string]any{
		"model": targetModel,
	}

	var messages []any
	if err := json.Unmarshal([]byte(cap.MessagesJSON), &messages); err == nil {
		payloadMap["messages"] = messages
	} else {
		payloadMap["messages"] = []map[string]string{
			{"role": "user", "content": cap.MessagesJSON},
		}
	}

	if fixedTemp != nil {
		payloadMap["temperature"] = *fixedTemp
	} else {
		payloadMap["temperature"] = 0.0
	}

	payloadBytes, _ := json.Marshal(payloadMap)
	reqURL := fmt.Sprintf("%s/v1/chat/completions", re.gatewayURL)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", reqURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return "", 0, 0, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if cap.VirtualKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+cap.VirtualKey)
	}

	start := time.Now()
	resp, err := re.httpClient.Do(httpReq)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return "", latency, 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", latency, 0, err
	}

	var respObj struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}

	content := string(body)
	if err := json.Unmarshal(body, &respObj); err == nil && len(respObj.Choices) > 0 {
		content = respObj.Choices[0].Message.Content
	}

	tokens := respObj.Usage.PromptTokens
	compTokens := respObj.Usage.CompletionTokens
	if tokens == 0 {
		tokens = cap.PromptTokens
		compTokens = cap.CompletionTokens
	}
	cost := EstimateCost(targetModel, tokens, compTokens)

	return content, latency, cost, nil
}
