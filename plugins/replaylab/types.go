package replaylab

import (
	"time"
)

// Verdict constants
const (
	VerdictSafeToSwitch       = "SAFE_TO_SWITCH"
	VerdictSwitchWithCaution  = "SWITCH_WITH_CAUTION"
	VerdictDoNotSwitch        = "DO_NOT_SWITCH"
)

// Experiment status constants
const (
	StatusPending   = "pending"
	StatusRunning   = "running"
	StatusCompleted = "completed"
	StatusCancelled = "cancelled"
	StatusFailed    = "failed"
)

// CaptureRecord represents a single recorded production gateway request/response pair.
type CaptureRecord struct {
	ID               string    `json:"id"`
	RequestID        string    `json:"request_id"`
	Timestamp        time.Time `json:"timestamp"`
	VirtualKey       string    `json:"virtual_key,omitempty"`
	Tenant           string    `json:"tenant,omitempty"`
	Provider         string    `json:"provider"`
	Model            string    `json:"model"`
	MessagesJSON     string    `json:"messages_json"`
	ParamsJSON       string    `json:"params_json,omitempty"`
	ResponseJSON     string    `json:"response_json,omitempty"`
	PromptTokens     int       `json:"prompt_tokens"`
	CompletionTokens int       `json:"completion_tokens"`
	TotalTokens      int       `json:"total_tokens"`
	LatencyMs        int64     `json:"latency_ms"`
	CostEstimate     float64   `json:"cost_estimate"`
	StatusCode       int       `json:"status_code"`
	ErrorMessage     string    `json:"error_message,omitempty"`
	IsStreaming      bool      `json:"is_streaming"`
	Tag              string    `json:"tag,omitempty"`
	HashOnly         bool      `json:"hash_only"`
}

// Config controls sampling, privacy, storage, and budget safety.
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
		GatewayURL:       "http://localhost:8080",
		DefaultBudgetCap: 50.0, // $50 max safety cap
	}
}

// Experiment represents a model comparison trial replaying captured traffic.
type Experiment struct {
	ID                string                 `json:"id"`
	Name              string                 `json:"name"`
	Status            string                 `json:"status"`
	OriginalModel     string                 `json:"original_model"`
	CandidateModels   []string               `json:"candidate_models"`
	FilterTag         string                 `json:"filter_tag,omitempty"`
	SampleSize        int                    `json:"sample_size"`
	BudgetCapDollars  float64                `json:"budget_cap_dollars"`
	FixedTemperature  *float64               `json:"fixed_temperature,omitempty"`
	DryRun            bool                   `json:"dry_run"`
	CreatedAt         time.Time              `json:"created_at"`
	CompletedAt       *time.Time             `json:"completed_at,omitempty"`
	TotalReplays      int                    `json:"total_replays"`
	CompletedReplays  int                    `json:"completed_replays"`
	SpentDollars      float64                `json:"spent_dollars"`
	SpentTokens       int                    `json:"spent_tokens"`
	Results           []ExperimentResult     `json:"results"`
	Recommendations   []SwitchRecommendation `json:"recommendations"`
	SummaryMarkdown   string                 `json:"summary_markdown,omitempty"`
}

// ExperimentResult summarizes comparison metrics for a single candidate model against the baseline.
type ExperimentResult struct {
	CandidateModel         string  `json:"candidate_model"`
	TotalRequests          int     `json:"total_requests"`
	SuccessfulRequests     int     `json:"successful_requests"`
	FailedRequests         int     `json:"failed_requests"`
	OriginalAvgLatencyMs   float64 `json:"original_avg_latency_ms"`
	CandidateAvgLatencyMs  float64 `json:"candidate_avg_latency_ms"`
	P50LatencyMs           float64 `json:"p50_latency_ms"`
	P95LatencyMs           float64 `json:"p95_latency_ms"`
	LatencyDeltaPercent    float64 `json:"latency_delta_percent"`
	OriginalCostDollars    float64 `json:"original_cost_dollars"`
	CandidateCostDollars   float64 `json:"candidate_cost_dollars"`
	CostSavingsDollars     float64 `json:"cost_savings_dollars"`
	CostSavingsPercent     float64 `json:"cost_savings_percent"`
	AvgQualityScore        float64 `json:"avg_quality_score"` // 0 - 100
	JSONValidityRate       float64 `json:"json_validity_rate"` // 0 - 100
	ErrorRate              float64 `json:"error_rate"`
	Recommendation         string  `json:"recommendation"`
	Reasoning              string  `json:"reasoning"`
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
}

// MetricsSnapshot exposes Prometheus-compatible telemetry numbers.
type MetricsSnapshot struct {
	CapturesTotal        int64   `json:"captures_total"`
	CapturesDroppedTotal int64   `json:"captures_dropped_total"`
	ReplaysTotal         int64   `json:"replays_total"`
	ReplaysFailedTotal   int64   `json:"replays_failed_total"`
	TotalDollarsSaved    float64 `json:"total_dollars_saved"`
}
