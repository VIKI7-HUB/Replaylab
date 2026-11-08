package replaylab

import (
	"fmt"
	"strings"
	"time"
)

// Verdict constants
const (
	VerdictSafeToSwitch      = "SAFE_TO_SWITCH"
	VerdictSwitchWithCaution = "SWITCH_WITH_CAUTION"
	VerdictDoNotSwitch       = "DO_NOT_SWITCH"
)

// Experiment status constants
const (
	StatusPending   = "pending"
	StatusRunning   = "running"
	StatusCompleted = "completed"
	StatusCancelled = "cancelled"
	StatusFailed    = "failed"
	// StatusBudgetExhausted means the run stopped early because the next replay
	// would have pushed spend past the budget cap. Partial results are kept.
	StatusBudgetExhausted = "budget_exhausted"
)

// IsTerminalStatus reports whether an experiment can no longer change.
func IsTerminalStatus(status string) bool {
	switch status {
	case StatusCompleted, StatusCancelled, StatusFailed, StatusBudgetExhausted:
		return true
	}
	return false
}

// CaptureRecord represents a single recorded production gateway request/response pair.
type CaptureRecord struct {
	ID           string    `json:"id"`
	RequestID    string    `json:"request_id"`
	Timestamp    time.Time `json:"timestamp"`
	VirtualKey   string    `json:"virtual_key,omitempty"`
	Tenant       string    `json:"tenant,omitempty"`
	Provider     string    `json:"provider"`
	Model        string    `json:"model"`
	MessagesJSON string    `json:"messages_json"`
	// ParamsJSON holds the non-message request parameters (max_tokens,
	// response_format, ...) so replays reproduce the original call.
	ParamsJSON string `json:"params_json,omitempty"`
	// ResponseJSON holds the assistant's response content (not the raw
	// provider envelope), which is what candidate responses are scored against.
	ResponseJSON     string  `json:"response_json,omitempty"`
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	TotalTokens      int     `json:"total_tokens"`
	LatencyMs        int64   `json:"latency_ms"`
	CostEstimate     float64 `json:"cost_estimate"`
	StatusCode       int     `json:"status_code"`
	ErrorMessage     string  `json:"error_message,omitempty"`
	IsStreaming      bool    `json:"is_streaming"`
	Tag              string  `json:"tag,omitempty"`
	HashOnly         bool    `json:"hash_only"`
}

// Config controls sampling, privacy, storage, and budget safety.
// DBPath, GatewayURL, MaxQueueSize, WorkerCount and APIListenAddr are fixed at startup; the
// rest can be changed at runtime through PUT /replaylab/config.
type Config struct {
	Enabled          bool     `json:"enabled"`
	SamplingRate     float64  `json:"sampling_rate"` // 0.0 to 1.0 (e.g. 0.20 = 20%)
	OptInKeys        []string `json:"opt_in_keys"`   // if set, only capture these virtual keys
	OptOutKeys       []string `json:"opt_out_keys"`  // skip capture for these keys
	RedactPII        bool     `json:"redact_pii"`
	StoreHashesOnly  bool     `json:"store_hashes_only"`
	RetentionDays    int      `json:"retention_days"`
	MaxQueueSize     int      `json:"max_queue_size"`
	WorkerCount      int      `json:"worker_count"`
	DBPath           string   `json:"db_path"`
	GatewayURL       string   `json:"gateway_url"`
	DefaultBudgetCap float64  `json:"default_budget_cap"`
	// ReplayAuthToken, when set, is sent as the bearer token for every
	// replay. It is never serialised.
	ReplayAuthToken string `json:"-"`
	// APIListenAddr, when set (e.g. ":8086"), serves the /replaylab API on its
	// own listener. Useful when embedded in Bifrost. Startup only.
	APIListenAddr string `json:"api_listen_addr,omitempty"`
}

// DefaultConfig provides production-ready, safe defaults.
func DefaultConfig() Config {
	return Config{
		Enabled:          true,
		SamplingRate:     1.0, // 100% for demo, configurable via API
		RedactPII:        true,
		StoreHashesOnly:  false,
		RetentionDays:    30,
		MaxQueueSize:     10000,
		WorkerCount:      4,
		DBPath:           "replaylab.db",
		GatewayURL:       "http://localhost:8081",
		DefaultBudgetCap: 50.0, // $50 max safety cap
	}
}

// Experiment represents a model comparison trial replaying captured traffic.
type Experiment struct {
	ID               string     `json:"id"`
	Name             string     `json:"name"`
	Status           string     `json:"status"`
	OriginalModel    string     `json:"original_model"`
	CandidateModels  []string   `json:"candidate_models"`
	FilterTag        string     `json:"filter_tag,omitempty"`
	SampleSize       int        `json:"sample_size"`
	BudgetCapDollars float64    `json:"budget_cap_dollars"`
	FixedTemperature *float64   `json:"fixed_temperature,omitempty"`
	DryRun           bool       `json:"dry_run"`
	CreatedAt        time.Time  `json:"created_at"`
	CompletedAt      *time.Time `json:"completed_at,omitempty"`
	// MonthlyRequestVolume is the traffic volume used to project savings:
	// either supplied by the caller or extrapolated from captured traffic.
	MonthlyRequestVolume int64                  `json:"monthly_request_volume"`
	StopReason           string                 `json:"stop_reason,omitempty"`
	TotalReplays         int                    `json:"total_replays"`
	CompletedReplays     int                    `json:"completed_replays"`
	SpentDollars         float64                `json:"spent_dollars"`
	SpentTokens          int                    `json:"spent_tokens"`
	Results              []ExperimentResult     `json:"results"`
	Recommendations      []SwitchRecommendation `json:"recommendations"`
	SummaryMarkdown      string                 `json:"summary_markdown,omitempty"`
}

// ExperimentResult summarizes comparison metrics for a single candidate model against the baseline.
type ExperimentResult struct {
	CandidateModel          string  `json:"candidate_model"`
	TotalRequests           int     `json:"total_requests"`
	SuccessfulRequests      int     `json:"successful_requests"`
	FailedRequests          int     `json:"failed_requests"`
	OriginalAvgLatencyMs    float64 `json:"original_avg_latency_ms"`
	CandidateAvgLatencyMs   float64 `json:"candidate_avg_latency_ms"`
	P50LatencyMs            float64 `json:"p50_latency_ms"`
	P95LatencyMs            float64 `json:"p95_latency_ms"`
	LatencyDeltaPercent     float64 `json:"latency_delta_percent"`
	OriginalCostDollars     float64 `json:"original_cost_dollars"`
	CandidateCostDollars    float64 `json:"candidate_cost_dollars"`
	CostSavingsDollars      float64 `json:"cost_savings_dollars"`
	CostSavingsPercent      float64 `json:"cost_savings_percent"`
	AvgQualityScore         float64 `json:"avg_quality_score"`  // 0 - 100
	JSONValidityRate        float64 `json:"json_validity_rate"` // 0 - 100, over JSONSamples
	JSONSamples             int     `json:"json_samples"`       // replays whose baseline response was JSON
	ErrorRate               float64 `json:"error_rate"`
	Recommendation          string  `json:"recommendation"`
	Reasoning               string  `json:"reasoning"`
	ProjectedMonthlySavings float64 `json:"projected_monthly_savings"`
}

// SwitchRecommendation encapsulates final decision guidance per model route.
type SwitchRecommendation struct {
	OriginalModel           string  `json:"original_model"`
	CandidateModel          string  `json:"candidate_model"`
	Verdict                 string  `json:"verdict"`
	Confidence              float64 `json:"confidence"`
	ProjectedMonthlySavings float64 `json:"projected_monthly_savings"`
	Reasoning               string  `json:"reasoning"`
}

// CreateExperimentRequest payload.
type CreateExperimentRequest struct {
	Name             string   `json:"name"`
	CandidateModels  []string `json:"candidate_models"`
	OriginalModel    string   `json:"original_model,omitempty"`
	FilterTag        string   `json:"filter_tag,omitempty"`
	SampleSize       int      `json:"sample_size"`
	BudgetCapDollars float64  `json:"budget_cap_dollars"`
	FixedTemperature *float64 `json:"fixed_temperature,omitempty"`
	DryRun           bool     `json:"dry_run"`
	// MonthlyRequestVolume overrides the volume used for savings projections.
	// When zero, it is extrapolated from the captured traffic rate.
	MonthlyRequestVolume int64 `json:"monthly_request_volume,omitempty"`
}

// Limits applied to experiment requests.
const (
	DefaultSampleSize = 50
	MaxSampleSize     = 1000
	MaxCandidates     = 10
)

// Normalize fills defaults and validates the request.
func (r *CreateExperimentRequest) Normalize(defaultBudget float64) error {
	r.Name = strings.TrimSpace(r.Name)
	if r.Name == "" {
		r.Name = "Untitled experiment"
	}
	seen := make(map[string]struct{}, len(r.CandidateModels))
	cands := r.CandidateModels[:0]
	for _, m := range r.CandidateModels {
		m = strings.TrimSpace(m)
		if m == "" {
			continue
		}
		if _, dup := seen[m]; dup {
			continue
		}
		seen[m] = struct{}{}
		cands = append(cands, m)
	}
	r.CandidateModels = cands
	if len(r.CandidateModels) == 0 {
		return fmt.Errorf("candidate_models must list at least one model")
	}
	if len(r.CandidateModels) > MaxCandidates {
		return fmt.Errorf("candidate_models is limited to %d models", MaxCandidates)
	}
	if r.SampleSize <= 0 {
		r.SampleSize = DefaultSampleSize
	}
	if r.SampleSize > MaxSampleSize {
		return fmt.Errorf("sample_size is limited to %d", MaxSampleSize)
	}
	if r.BudgetCapDollars < 0 {
		return fmt.Errorf("budget_cap_dollars must not be negative")
	}
	if r.BudgetCapDollars == 0 {
		r.BudgetCapDollars = defaultBudget
	}
	if r.FixedTemperature != nil && (*r.FixedTemperature < 0 || *r.FixedTemperature > 2) {
		return fmt.Errorf("fixed_temperature must be between 0 and 2")
	}
	if r.MonthlyRequestVolume < 0 {
		return fmt.Errorf("monthly_request_volume must not be negative")
	}
	return nil
}

// Validate checks the runtime-adjustable fields.
func (c Config) Validate() error {
	if c.SamplingRate < 0 || c.SamplingRate > 1 {
		return fmt.Errorf("sampling_rate must be between 0 and 1")
	}
	if c.RetentionDays < 0 {
		return fmt.Errorf("retention_days must not be negative")
	}
	if c.DefaultBudgetCap < 0 {
		return fmt.Errorf("default_budget_cap must not be negative")
	}
	return nil
}
