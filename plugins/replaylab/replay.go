package replaylab

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

// ReplayEvent is a progress update streamed to SSE subscribers.
type ReplayEvent struct {
	ExperimentID    string  `json:"experiment_id"`
	Status          string  `json:"status"`
	CandidateModel  string  `json:"candidate_model,omitempty"`
	CompletedCount  int     `json:"completed_count"`
	TotalCount      int     `json:"total_count"`
	ProgressPercent float64 `json:"progress_percent"`
	CurrentQuality  float64 `json:"current_quality"`
	// CurrentJudgeScore is the running judge score of the current candidate,
	// omitted until one of its replays has been judged.
	CurrentJudgeScore *float64 `json:"current_judge_score,omitempty"`
	SpentDollars      float64  `json:"spent_dollars"`
	Message           string   `json:"message"`
	Final             bool     `json:"final"`
}

const (
	maxUpstreamBody     = 4 << 20 // 4 MiB cap on a replayed response body
	progressSaveEvery   = 10      // persist progress every N replays
	replayPacing        = 10 * time.Millisecond
	errorExcerptLength  = 200
	defaultReplayBudget = 50.0
)

// ReplayEngine replays captured traffic against candidate models while
// enforcing a hard budget cap.
type ReplayEngine struct {
	storage    Storage
	scorer     *QualityScorer
	gatewayURL string
	authToken  string
	httpClient *http.Client

	runsMu     sync.Mutex
	activeRuns map[string]context.CancelFunc

	subsMu sync.Mutex
	subs   map[string]map[chan ReplayEvent]struct{}

	replaysTotal  atomic.Int64
	replaysFailed atomic.Int64
}

// NewReplayEngine creates a ReplayEngine that sends replays to the
// OpenAI-compatible endpoint at gatewayURL.
func NewReplayEngine(storage Storage, gatewayURL string) *ReplayEngine {
	if gatewayURL == "" {
		gatewayURL = DefaultConfig().GatewayURL
	}
	return &ReplayEngine{
		storage:    storage,
		scorer:     NewQualityScorer(),
		gatewayURL: strings.TrimRight(gatewayURL, "/"),
		httpClient: &http.Client{Timeout: 60 * time.Second},
		activeRuns: make(map[string]context.CancelFunc),
		subs:       make(map[string]map[chan ReplayEvent]struct{}),
	}
}

// SetAuthToken sets the bearer token used for replays. When empty, each
// replay reuses the virtual key recorded with its capture.
func (re *ReplayEngine) SetAuthToken(token string) {
	re.authToken = token
}

// Metrics returns lifetime replay and failure counts.
func (re *ReplayEngine) Metrics() (total, failed int64) {
	return re.replaysTotal.Load(), re.replaysFailed.Load()
}

// ActiveExperiments returns how many experiments are currently running.
func (re *ReplayEngine) ActiveExperiments() int {
	re.runsMu.Lock()
	defer re.runsMu.Unlock()
	return len(re.activeRuns)
}

// SubscribeEvents registers a listener for an experiment's progress. The
// channel is closed when the experiment reaches a terminal state; callers
// that stop listening earlier must call the returned unsubscribe function.
func (re *ReplayEngine) SubscribeEvents(expID string) (<-chan ReplayEvent, func()) {
	ch := make(chan ReplayEvent, 100)
	re.subsMu.Lock()
	if re.subs[expID] == nil {
		re.subs[expID] = make(map[chan ReplayEvent]struct{})
	}
	re.subs[expID][ch] = struct{}{}
	re.subsMu.Unlock()

	unsubscribe := func() {
		re.subsMu.Lock()
		defer re.subsMu.Unlock()
		if set, ok := re.subs[expID]; ok {
			if _, ok := set[ch]; ok {
				delete(set, ch)
				close(ch)
			}
			if len(set) == 0 {
				delete(re.subs, expID)
			}
		}
	}
	return ch, unsubscribe
}

// broadcast sends an event to every subscriber without blocking; a slow
// subscriber misses intermediate progress events rather than stalling the run.
func (re *ReplayEngine) broadcast(ev ReplayEvent) {
	re.subsMu.Lock()
	defer re.subsMu.Unlock()
	for ch := range re.subs[ev.ExperimentID] {
		select {
		case ch <- ev:
		default:
		}
	}
}

// finish delivers the final event and closes all subscriber channels.
func (re *ReplayEngine) finish(ev ReplayEvent) {
	re.subsMu.Lock()
	defer re.subsMu.Unlock()
	for ch := range re.subs[ev.ExperimentID] {
		select {
		case ch <- ev:
		default:
			// Buffer full: drop the oldest event so the final one always lands.
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- ev:
			default:
			}
		}
		close(ch)
	}
	delete(re.subs, ev.ExperimentID)
}

// CancelExperiment stops a running experiment. It returns false when the
// experiment is not running.
func (re *ReplayEngine) CancelExperiment(expID string) bool {
	re.runsMu.Lock()
	cancel, ok := re.activeRuns[expID]
	re.runsMu.Unlock()
	if ok {
		cancel()
	}
	return ok
}

func (re *ReplayEngine) selectCaptures(req CreateExperimentRequest) ([]*CaptureRecord, error) {
	captures, err := re.storage.GetCaptures(CaptureFilter{
		Model:      req.OriginalModel,
		Tag:        req.FilterTag,
		Streamed:   req.Streamed,
		Limit:      req.SampleSize,
		Replayable: true,
	})
	if err != nil {
		return nil, err
	}
	if len(captures) == 0 {
		target := req.OriginalModel
		if target == "" {
			target = "any model"
		}
		return nil, fmt.Errorf("no replayable captures found for %s (captures must be successful and not stored as hashes)", target)
	}
	return captures, nil
}

func (re *ReplayEngine) newExperiment(req CreateExperimentRequest, captures []*CaptureRecord) *Experiment {
	origModel := req.OriginalModel
	if origModel == "" {
		origModel = captures[0].Model
	}
	exp := &Experiment{
		ID:                   "exp-" + uuid.NewString()[:10],
		Name:                 req.Name,
		Status:               StatusPending,
		OriginalModel:        origModel,
		CandidateModels:      req.CandidateModels,
		FilterTag:            req.FilterTag,
		Streamed:             req.Streamed,
		SampleSize:           len(captures),
		BudgetCapDollars:     req.BudgetCapDollars,
		FixedTemperature:     req.FixedTemperature,
		DryRun:               req.DryRun,
		CreatedAt:            time.Now().UTC(),
		TotalReplays:         len(captures) * len(req.CandidateModels),
		MonthlyRequestVolume: req.MonthlyRequestVolume,
		Judge:                req.Judge,
		Results:              []ExperimentResult{},
		Recommendations:      []SwitchRecommendation{},
	}
	if req.Judge != nil {
		judge := *req.Judge
		exp.Judge = &judge
		_, known := LookupPricing(judge.Model)
		exp.JudgePriceEstimated = !known
	}
	if exp.MonthlyRequestVolume == 0 {
		if vol, err := re.storage.GetTrafficVolume(origModel); err == nil {
			exp.MonthlyRequestVolume = vol.MonthlyEstimate()
		}
	}
	return exp
}

// EstimateDryRun projects the replay count and cost without calling any model.
// The estimate assumes candidates produce as many tokens as the baseline did.
func (re *ReplayEngine) EstimateDryRun(req CreateExperimentRequest) (*Experiment, error) {
	captures, err := re.selectCaptures(req)
	if err != nil {
		return nil, err
	}
	exp := re.newExperiment(req, captures)
	exp.DryRun = true

	var replayCost, judgeCost float64
	for _, cand := range req.CandidateModels {
		for i, c := range captures {
			replayCost += EstimateCost(cand, c.PromptTokens, c.CompletionTokens)
			if req.Judge != nil && (req.Judge.MaxJudgeSamples <= 0 || i < req.Judge.MaxJudgeSamples) {
				judgeCost += projectedJudgeCost(req.Judge.Model, c)
			}
		}
	}
	estimated := replayCost + judgeCost
	exp.SpentDollars = estimated
	if req.Judge != nil {
		exp.EstimatedReplayDollars, exp.EstimatedJudgeDollars = &replayCost, &judgeCost
	}
	if estimated > exp.BudgetCapDollars {
		exp.StopReason = fmt.Sprintf("estimated cost $%.4f exceeds budget cap $%.2f; the run would stop early", estimated, exp.BudgetCapDollars)
	}
	return exp, nil
}

// StartExperiment validates the request and begins replaying asynchronously.
// The returned experiment is a snapshot; poll storage or subscribe to events
// for progress.
func (re *ReplayEngine) StartExperiment(req CreateExperimentRequest) (*Experiment, error) {
	if err := req.Normalize(defaultReplayBudget); err != nil {
		return nil, err
	}
	if req.DryRun {
		return re.EstimateDryRun(req)
	}

	captures, err := re.selectCaptures(req)
	if err != nil {
		return nil, err
	}
	exp := re.newExperiment(req, captures)
	exp.Status = StatusRunning

	if err := re.storage.SaveExperiment(exp); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	re.runsMu.Lock()
	re.activeRuns[exp.ID] = cancel
	re.runsMu.Unlock()

	snapshot := *exp // the goroutine owns exp from here on
	go re.run(ctx, exp, captures)
	return &snapshot, nil
}

// candidateRun accumulates replay outcomes for one candidate model.
type candidateRun struct {
	model          string
	attempted      int
	successful     int
	failed         int
	candLatencies  []float64
	origLatencies  []float64
	qualitySum     float64
	jsonSamples    int
	jsonValid      int
	origCost       float64
	candCost       float64
	lastErrMessage string

	judged        bool // the experiment has a judge
	judgeAttempts int  // successful replays sent to the judge (for max_judge_samples)
	judgedReplays int
	judgeScoreSum float64
	judgeWins     int
	judgeTies     int
	judgeLosses   int
	disagreements int
	judgeCost     float64
	judgeSamples  []JudgementSample
}

// wantsJudge reports whether the next successful replay should be judged.
func (c *candidateRun) wantsJudge(cfg *JudgeConfig) bool {
	return cfg != nil && (cfg.MaxJudgeSamples <= 0 || c.judgeAttempts < cfg.MaxJudgeSamples)
}

// runningJudgeScore is the mean judge score so far, or nil before any judgement.
func (c *candidateRun) runningJudgeScore() *float64 {
	if c.judgedReplays == 0 {
		return nil
	}
	s := c.judgeScoreSum / float64(c.judgedReplays)
	return &s
}

func (c *candidateRun) addJudgement(j replayJudgement, capture *CaptureRecord, candidate string) {
	c.judgeAttempts++
	c.judgeCost += j.cost
	if !j.ok {
		return
	}
	if len(c.judgeSamples) < MaxStoredJudgements {
		c.judgeSamples = append(c.judgeSamples, j.sample(capture, candidate))
	}
	c.judgedReplays++
	c.judgeScoreSum += j.score()
	for _, o := range j.outcomes {
		switch o {
		case judgeWin:
			c.judgeWins++
		case judgeTie:
			c.judgeTies++
		default:
			c.judgeLosses++
		}
	}
	if j.selfDisagreement() {
		c.disagreements++
	}
}

// fillJudge sets the judge fields of a result; it leaves them nil when the
// experiment has no judge.
func (c *candidateRun) fillJudge(res *ExperimentResult) {
	if !c.judged {
		return
	}
	wins, ties, losses := c.judgeWins, c.judgeTies, c.judgeLosses
	res.JudgeWins, res.JudgeTies, res.JudgeLosses = &wins, &ties, &losses
	coverage := 0.0
	if c.successful > 0 {
		coverage = float64(c.judgedReplays) / float64(c.successful) * 100.0
	}
	res.JudgeCoverage = &coverage
	spent := c.judgeCost
	res.JudgeSpentDollars = &spent
	res.JudgeSamples = c.judgeSamples
	if c.judgedReplays > 0 {
		score := c.judgeScoreSum / float64(c.judgedReplays)
		disagreement := float64(c.disagreements) / float64(c.judgedReplays) * 100.0
		res.JudgeScore, res.JudgeDisagreementRate = &score, &disagreement
	}
}

func (c *candidateRun) avgQuality() float64 {
	if c.successful == 0 {
		return 0
	}
	return c.qualitySum / float64(c.successful)
}

func (c *candidateRun) result() ExperimentResult {
	res := ExperimentResult{
		CandidateModel:     c.model,
		TotalRequests:      c.attempted,
		SuccessfulRequests: c.successful,
		FailedRequests:     c.failed,
		JSONSamples:        c.jsonSamples,
		JSONValidityRate:   100,
	}
	if c.attempted > 0 {
		res.ErrorRate = float64(c.failed) / float64(c.attempted) * 100.0
	}
	c.fillJudge(&res)
	if c.successful == 0 {
		res.AvgQualityScore = 0
		return res
	}
	res.OriginalCostDollars = c.origCost
	res.CandidateCostDollars = c.candCost
	res.CostSavingsDollars = c.origCost - c.candCost
	if c.origCost > 0 {
		res.CostSavingsPercent = res.CostSavingsDollars / c.origCost * 100.0
	}
	res.P50LatencyMs, res.P95LatencyMs = ComputePercentiles(c.candLatencies)
	res.OriginalAvgLatencyMs = mean(c.origLatencies)
	res.CandidateAvgLatencyMs = mean(c.candLatencies)
	if res.OriginalAvgLatencyMs > 0 {
		res.LatencyDeltaPercent = (res.CandidateAvgLatencyMs - res.OriginalAvgLatencyMs) / res.OriginalAvgLatencyMs * 100.0
	}
	res.AvgQualityScore = c.avgQuality()
	if c.jsonSamples > 0 {
		res.JSONValidityRate = float64(c.jsonValid) / float64(c.jsonSamples) * 100.0
	}
	return res
}

func mean(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	var sum float64
	for _, x := range xs {
		sum += x
	}
	return sum / float64(len(xs))
}

func (re *ReplayEngine) run(ctx context.Context, exp *Experiment, captures []*CaptureRecord) {
	defer func() {
		re.runsMu.Lock()
		delete(re.activeRuns, exp.ID)
		re.runsMu.Unlock()
	}()
	defer func() {
		if r := recover(); r != nil {
			log.Printf("replaylab: experiment %s panicked: %v", exp.ID, r)
			exp.Status = StatusFailed
			exp.StopReason = fmt.Sprintf("internal error: %v", r)
			re.finalize(exp, nil)
		}
	}()

	runs := make([]*candidateRun, 0, len(exp.CandidateModels))

	for _, model := range exp.CandidateModels {
		run := &candidateRun{model: model, judged: exp.Judge != nil}
		runs = append(runs, run)

		for _, c := range captures {
			if ctx.Err() != nil {
				exp.Status = StatusCancelled
				exp.StopReason = "cancelled by operator"
				re.finalize(exp, runs)
				return
			}

			// Hard budget cap: refuse any replay whose estimated cost
			// (assuming baseline-sized token usage, plus both judge calls when
			// judging) would cross the cap.
			judgeThis := run.wantsJudge(exp.Judge)
			projected := EstimateCost(model, c.PromptTokens, c.CompletionTokens)
			if judgeThis {
				projected += projectedJudgeCost(exp.Judge.Model, c)
			}
			if exp.SpentDollars+projected > exp.BudgetCapDollars {
				re.stopForBudget(exp, runs)
				return
			}
			judgeBudgetHit := false

			out := re.replayOne(ctx, model, c, exp.FixedTemperature)
			if out.err != nil && ctx.Err() != nil {
				continue // interrupted by cancellation; handled at the top of the loop
			}
			run.attempted++
			exp.CompletedReplays++
			re.replaysTotal.Add(1)

			if out.err != nil {
				run.failed++
				run.lastErrMessage = out.err.Error()
				re.replaysFailed.Add(1)
			} else {
				q := re.scorer.Score(c.ResponseJSON, out.content)
				run.successful++
				run.qualitySum += q.Score
				if q.BaselineJSON {
					run.jsonSamples++
					if q.CandidateJSONValid {
						run.jsonValid++
					}
				}
				run.candLatencies = append(run.candLatencies, float64(out.latencyMs))
				run.origLatencies = append(run.origLatencies, float64(c.LatencyMs))
				run.origCost += c.CostEstimate
				run.candCost += out.cost
				exp.SpentDollars += out.cost
				exp.SpentTokens += out.tokens

				if judgeThis {
					j := re.judgeReplay(ctx, exp.Judge, c, out.content, exp.BudgetCapDollars-exp.SpentDollars)
					run.addJudgement(j, c, out.content)
					exp.SpentDollars += j.cost
					exp.SpentTokens += j.tokens
					judgeBudgetHit = j.budgetHit
				}
			}

			re.broadcast(ReplayEvent{
				ExperimentID:      exp.ID,
				Status:            exp.Status,
				CandidateModel:    model,
				CompletedCount:    exp.CompletedReplays,
				TotalCount:        exp.TotalReplays,
				ProgressPercent:   float64(exp.CompletedReplays) / float64(exp.TotalReplays) * 100.0,
				CurrentQuality:    run.avgQuality(),
				CurrentJudgeScore: run.runningJudgeScore(),
				SpentDollars:      exp.SpentDollars,
				Message:           fmt.Sprintf("Replayed %s -> %s", c.ID, model),
			})

			if judgeBudgetHit {
				// The replay landed but judging it could have crossed the cap.
				re.stopForBudget(exp, runs)
				return
			}

			if exp.CompletedReplays%progressSaveEvery == 0 {
				if err := re.storage.SaveExperiment(exp); err != nil {
					log.Printf("replaylab: failed to save progress for %s: %v", exp.ID, err)
				}
			}

			select {
			case <-ctx.Done():
			case <-time.After(replayPacing):
			}
		}
	}

	exp.Status = StatusCompleted
	re.finalize(exp, runs)
}

func (re *ReplayEngine) stopForBudget(exp *Experiment, runs []*candidateRun) {
	exp.Status = StatusBudgetExhausted
	exp.StopReason = fmt.Sprintf("stopped before exceeding the $%.2f budget cap (spent $%.4f)", exp.BudgetCapDollars, exp.SpentDollars)
	re.finalize(exp, runs)
}

// finalize computes results for every candidate that ran, persists the
// experiment in its terminal state, and closes the event stream.
func (re *ReplayEngine) finalize(exp *Experiment, runs []*candidateRun) {
	exp.Results = []ExperimentResult{}
	exp.Recommendations = []SwitchRecommendation{}
	if exp.Judge != nil {
		var judgeSpent float64
		for _, r := range runs {
			judgeSpent += r.judgeCost
		}
		exp.JudgeSpentDollars = &judgeSpent
	}
	for _, r := range runs {
		if r.attempted == 0 {
			continue
		}
		res := r.result()
		rec := GenerateRecommendation(exp.OriginalModel, r.model, &res, exp.MonthlyRequestVolume)
		if r.failed > 0 && r.lastErrMessage != "" {
			res.Reasoning += "; last error: " + truncate(r.lastErrMessage, errorExcerptLength)
		}
		exp.Results = append(exp.Results, res)
		exp.Recommendations = append(exp.Recommendations, rec)
	}

	now := time.Now().UTC()
	exp.CompletedAt = &now
	exp.SummaryMarkdown = FormatMarkdownReport(exp)
	if err := re.storage.SaveExperiment(exp); err != nil {
		log.Printf("replaylab: failed to save final state for %s: %v", exp.ID, err)
	}

	msg := "Experiment completed"
	if exp.StopReason != "" {
		msg = "Experiment " + exp.Status + ": " + exp.StopReason
	}
	progress := 100.0
	if exp.TotalReplays > 0 {
		progress = float64(exp.CompletedReplays) / float64(exp.TotalReplays) * 100.0
	}
	re.finish(ReplayEvent{
		ExperimentID:    exp.ID,
		Status:          exp.Status,
		CompletedCount:  exp.CompletedReplays,
		TotalCount:      exp.TotalReplays,
		ProgressPercent: progress,
		SpentDollars:    exp.SpentDollars,
		Message:         msg,
		Final:           true,
	})
}

type replayOutcome struct {
	content   string
	latencyMs int64
	cost      float64
	tokens    int
	err       error
}

// buildReplayPayload reconstructs an OpenAI-compatible chat request from a
// capture, preserving its original parameters but swapping the model.
func buildReplayPayload(targetModel string, c *CaptureRecord, fixedTemp *float64) ([]byte, error) {
	payload := map[string]any{}
	if c.ParamsJSON != "" {
		if err := json.Unmarshal([]byte(c.ParamsJSON), &payload); err != nil {
			payload = map[string]any{}
		}
	}

	var messages []any
	if err := json.Unmarshal([]byte(c.MessagesJSON), &messages); err == nil && len(messages) > 0 {
		payload["messages"] = messages
	} else {
		payload["messages"] = []map[string]string{{"role": "user", "content": c.MessagesJSON}}
	}

	payload["model"] = targetModel
	// Replays are never streamed, so streamed captures score on the whole
	// answer; stream_options is only valid alongside stream: true.
	payload["stream"] = false
	delete(payload, "stream_options")
	switch {
	case fixedTemp != nil:
		payload["temperature"] = *fixedTemp
	case payload["temperature"] == nil:
		payload["temperature"] = 0.0 // deterministic by default for fair comparison
	}
	return json.Marshal(payload)
}

func (re *ReplayEngine) replayOne(ctx context.Context, targetModel string, c *CaptureRecord, fixedTemp *float64) replayOutcome {
	body, err := buildReplayPayload(targetModel, c, fixedTemp)
	if err != nil {
		return replayOutcome{err: err}
	}

	parsed, latency, err := re.postChat(ctx, body, re.replayToken(c))
	if err != nil {
		return replayOutcome{latencyMs: latency, err: err}
	}

	prompt, completion := parsed.promptTokens, parsed.completionTokens
	if prompt == 0 && completion == 0 {
		// Provider omitted usage; fall back to the baseline's token counts.
		prompt, completion = c.PromptTokens, c.CompletionTokens
	}
	return replayOutcome{
		content:   parsed.content,
		latencyMs: latency,
		cost:      EstimateCost(targetModel, prompt, completion),
		tokens:    prompt + completion,
	}
}

// postChat sends a non-streaming chat completion through the gateway and
// returns the parsed response and the call's latency in milliseconds.
func (re *ReplayEngine) postChat(ctx context.Context, body []byte, token string) (chatCompletion, int64, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, re.gatewayURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return chatCompletion{}, 0, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Replaylab-Replay", "1") // lets a capturing proxy skip replays and judge calls
	if token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+token)
	}

	start := time.Now()
	resp, err := re.httpClient.Do(httpReq)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return chatCompletion{}, latency, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxUpstreamBody))
	if err != nil {
		return chatCompletion{}, latency, err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return chatCompletion{}, latency, fmt.Errorf("upstream returned HTTP %d: %s", resp.StatusCode, truncate(string(raw), errorExcerptLength))
	}
	parsed, err := parseChatCompletion(raw)
	return parsed, latency, err
}

// replayToken picks the credential for a replay: the configured token, else
// the capture's virtual key unless redaction replaced it.
func (re *ReplayEngine) replayToken(c *CaptureRecord) string {
	if re.authToken != "" {
		return re.authToken
	}
	if c.VirtualKey != "" && !strings.Contains(c.VirtualKey, "[REDACTED") {
		return c.VirtualKey
	}
	return ""
}

type chatCompletion struct {
	content          string
	promptTokens     int
	completionTokens int
}

var errNoChoices = errors.New("upstream response contained no choices")

// parseChatCompletion extracts the assistant content and usage from an
// OpenAI-compatible chat completion response.
func parseChatCompletion(raw []byte) (chatCompletion, error) {
	var resp struct {
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
	if err := json.Unmarshal(raw, &resp); err != nil {
		return chatCompletion{}, fmt.Errorf("upstream response is not valid JSON: %w", err)
	}
	if len(resp.Choices) == 0 {
		return chatCompletion{}, errNoChoices
	}
	return chatCompletion{
		content:          resp.Choices[0].Message.Content,
		promptTokens:     resp.Usage.PromptTokens,
		completionTokens: resp.Usage.CompletionTokens,
	}, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
