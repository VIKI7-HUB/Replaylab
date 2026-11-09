package replaylab

import (
	"context"
	"log"
	"math/rand/v2"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

// ModelPricing defines input and output cost per 1M tokens in USD.
type ModelPricing struct {
	InputPer1M  float64
	OutputPer1M float64
}

// KnownPricing is the list-price book (USD per 1M tokens) used for cost
// estimates. Keys are bare model names; provider prefixes ("openai/gpt-4o")
// and dated snapshots ("gpt-4o-2024-08-06") resolve to these entries.
var KnownPricing = map[string]ModelPricing{
	"gpt-4o":             {InputPer1M: 2.50, OutputPer1M: 10.00},
	"gpt-4o-mini":        {InputPer1M: 0.15, OutputPer1M: 0.60},
	"gpt-4.1":            {InputPer1M: 2.00, OutputPer1M: 8.00},
	"gpt-4.1-mini":       {InputPer1M: 0.40, OutputPer1M: 1.60},
	"gpt-4.1-nano":       {InputPer1M: 0.10, OutputPer1M: 0.40},
	"gpt-4":              {InputPer1M: 30.00, OutputPer1M: 60.00},
	"gpt-3.5-turbo":      {InputPer1M: 0.50, OutputPer1M: 1.50},
	"claude-3-5-sonnet":  {InputPer1M: 3.00, OutputPer1M: 15.00},
	"claude-3-5-haiku":   {InputPer1M: 0.80, OutputPer1M: 4.00},
	"claude-3-haiku":     {InputPer1M: 0.25, OutputPer1M: 1.25},
	"claude-3-opus":      {InputPer1M: 15.00, OutputPer1M: 75.00},
	"deepseek-chat":      {InputPer1M: 0.14, OutputPer1M: 0.28},
	"meta-llama-3.1-70b": {InputPer1M: 0.40, OutputPer1M: 0.40},
	"meta-llama-3.1-8b":  {InputPer1M: 0.05, OutputPer1M: 0.05},
}

// fallbackPricing is used for models missing from KnownPricing.
var fallbackPricing = ModelPricing{InputPer1M: 1.00, OutputPer1M: 3.00}

// LookupPricing resolves a model name to its price entry. The boolean is false
// when the fallback rate was used.
func LookupPricing(model string) (ModelPricing, bool) {
	name := strings.ToLower(strings.TrimSpace(model))
	if p, ok := KnownPricing[name]; ok {
		return p, true
	}
	// Strip a Bifrost-style provider prefix: "openai/gpt-4o" -> "gpt-4o".
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
		if p, ok := KnownPricing[name]; ok {
			return p, true
		}
	}
	// Dated or suffixed snapshots: pick the longest known prefix.
	best := ""
	for k := range KnownPricing {
		if strings.HasPrefix(name, k+"-") && len(k) > len(best) {
			best = k
		}
	}
	if best != "" {
		return KnownPricing[best], true
	}
	return fallbackPricing, false
}

// EstimateCost calculates approximate USD expenditure from token usage.
func EstimateCost(model string, promptTokens, completionTokens int) float64 {
	pricing, _ := LookupPricing(model)
	inputCost := (float64(promptTokens) / 1_000_000.0) * pricing.InputPer1M
	outputCost := (float64(completionTokens) / 1_000_000.0) * pricing.OutputPer1M
	return inputCost + outputCost
}

const (
	captureBatchSize     = 64
	captureFlushInterval = 250 * time.Millisecond
)

// CaptureQueue manages asynchronous, non-blocking request recording.
type CaptureQueue struct {
	storage       Storage
	redactor      *Redactor
	config        atomic.Pointer[Config]
	queue         chan *CaptureRecord
	capturesTotal atomic.Int64
	droppedTotal  atomic.Int64
	writeErrors   atomic.Int64
	// streamedTotal counts streamed captures accepted into the queue;
	// streamsTruncated counts streams that overflowed the per-stream cap.
	streamedTotal    atomic.Int64
	streamsTruncated atomic.Int64
	wg               sync.WaitGroup
	ctx              context.Context
	cancel           context.CancelFunc
}

// NewCaptureQueue creates and initializes worker routines.
func NewCaptureQueue(cfg Config, storage Storage) *CaptureQueue {
	queueSize := cfg.MaxQueueSize
	if queueSize <= 0 {
		queueSize = 10000
	}
	workerCount := cfg.WorkerCount
	if workerCount <= 0 {
		workerCount = 4
	}

	ctx, cancel := context.WithCancel(context.Background())
	cq := &CaptureQueue{
		storage:  storage,
		redactor: NewRedactor(true),
		queue:    make(chan *CaptureRecord, queueSize),
		ctx:      ctx,
		cancel:   cancel,
	}
	cq.config.Store(&cfg)

	for i := 0; i < workerCount; i++ {
		cq.wg.Add(1)
		go cq.worker()
	}

	return cq
}

// Config returns the configuration currently in effect.
func (cq *CaptureQueue) Config() Config {
	return *cq.config.Load()
}

// UpdateConfig swaps in a new runtime configuration. It takes effect for the
// next sampling decision and the next flushed batch.
func (cq *CaptureQueue) UpdateConfig(cfg Config) {
	cq.config.Store(&cfg)
}

// ShouldCapture evaluates sampling rate and opt-in/out policies.
func (cq *CaptureQueue) ShouldCapture(virtualKey string) bool {
	cfg := cq.config.Load()
	if !cfg.Enabled {
		return false
	}
	if slices.Contains(cfg.OptOutKeys, virtualKey) {
		return false
	}
	if len(cfg.OptInKeys) > 0 && !slices.Contains(cfg.OptInKeys, virtualKey) {
		return false
	}
	if cfg.SamplingRate < 1.0 && rand.Float64() >= cfg.SamplingRate {
		return false
	}
	return true
}

// Enqueue pushes a capture record into the buffer without ever blocking:
// when the buffer is full the record is dropped and counted.
func (cq *CaptureQueue) Enqueue(rec *CaptureRecord) bool {
	if rec == nil {
		return false
	}
	if rec.ID == "" {
		rec.ID = "cap-" + uuid.NewString()[:12]
	}

	select {
	case cq.queue <- rec:
		cq.capturesTotal.Add(1)
		return true
	default:
		cq.droppedTotal.Add(1)
		return false
	}
}

// scrub applies hash-only or PII redaction according to the current config.
func (cq *CaptureQueue) scrub(r *CaptureRecord) {
	cfg := cq.config.Load()
	switch {
	case cfg.StoreHashesOnly:
		r.MessagesJSON = HashString(r.MessagesJSON)
		r.ResponseJSON = HashString(r.ResponseJSON)
		r.ParamsJSON = ""
		r.HashOnly = true
	case cfg.RedactPII:
		r.MessagesJSON = cq.redactor.Redact(r.MessagesJSON)
		r.ResponseJSON = cq.redactor.Redact(r.ResponseJSON)
		r.ErrorMessage = cq.redactor.Redact(r.ErrorMessage)
	}
	// Never persist the raw credential used for the request.
	r.VirtualKey = cq.redactor.Redact(r.VirtualKey)
}

// worker batches records and flushes them to storage.
func (cq *CaptureQueue) worker() {
	defer cq.wg.Done()

	batch := make([]*CaptureRecord, 0, captureBatchSize)
	ticker := time.NewTicker(captureFlushInterval)
	defer ticker.Stop()

	flush := func() {
		if len(batch) == 0 {
			return
		}
		for _, r := range batch {
			cq.scrub(r)
		}
		if err := cq.storage.SaveCapturesBatch(batch); err != nil {
			cq.writeErrors.Add(int64(len(batch)))
			log.Printf("replaylab: failed to persist %d captures: %v", len(batch), err)
		}
		batch = batch[:0]
	}

	add := func(rec *CaptureRecord) {
		batch = append(batch, rec)
		if len(batch) >= captureBatchSize {
			flush()
		}
	}

	for {
		select {
		case <-cq.ctx.Done():
			// Drain whatever is still buffered before exiting.
			for {
				select {
				case rec := <-cq.queue:
					add(rec)
				default:
					flush()
					return
				}
			}
		case rec := <-cq.queue:
			add(rec)
		case <-ticker.C:
			flush()
		}
	}
}

// Stop drains the queue and waits for workers to exit.
func (cq *CaptureQueue) Stop() {
	cq.cancel()
	cq.wg.Wait()
}

// GetMetrics returns capture and drop counts.
func (cq *CaptureQueue) GetMetrics() (captured int64, dropped int64) {
	return cq.capturesTotal.Load(), cq.droppedTotal.Load()
}

// StreamMetrics returns how many streamed captures were queued and how many
// streams were truncated by the per-stream content cap.
func (cq *CaptureQueue) StreamMetrics() (streamed int64, truncated int64) {
	return cq.streamedTotal.Load(), cq.streamsTruncated.Load()
}

// WriteErrors returns how many captures failed to persist.
func (cq *CaptureQueue) WriteErrors() int64 {
	return cq.writeErrors.Load()
}
