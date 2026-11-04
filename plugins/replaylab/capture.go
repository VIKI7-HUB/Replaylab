package replaylab

import (
	"context"
	"math/rand"
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

// Known model price book
var KnownPricing = map[string]ModelPricing{
	"gpt-4o":               {InputPer1M: 2.50, OutputPer1M: 10.00},
	"gpt-4o-mini":          {InputPer1M: 0.15, OutputPer1M: 0.60},
	"gpt-4":                {InputPer1M: 30.00, OutputPer1M: 60.00},
	"gpt-3.5-turbo":        {InputPer1M: 0.50, OutputPer1M: 1.50},
	"claude-3-5-sonnet":    {InputPer1M: 3.00, OutputPer1M: 15.00},
	"claude-3-haiku":       {InputPer1M: 0.25, OutputPer1M: 1.25},
	"claude-3-opus":        {InputPer1M: 15.00, OutputPer1M: 75.00},
	"deepseek-chat":        {InputPer1M: 0.14, OutputPer1M: 0.28},
	"meta-llama-3.1-70b":   {InputPer1M: 0.40, OutputPer1M: 0.40},
	"meta-llama-3.1-8b":    {InputPer1M: 0.05, OutputPer1M: 0.05},
}

// EstimateCost calculates approximate USD expenditure from token usage.
func EstimateCost(model string, promptTokens, completionTokens int) float64 {
	pricing, ok := KnownPricing[model]
	if !ok {
		// Default to moderate rate if model is unlisted
		pricing = ModelPricing{InputPer1M: 1.00, OutputPer1M: 3.00}
	}
	inputCost := (float64(promptTokens) / 1000000.0) * pricing.InputPer1M
	outputCost := (float64(completionTokens) / 1000000.0) * pricing.OutputPer1M
	return inputCost + outputCost
}

// CaptureQueue manages asynchronous, zero-blocking request recording.
type CaptureQueue struct {
	storage       Storage
	redactor      *Redactor
	config        Config
	queue         chan *CaptureRecord
	capturesTotal atomic.Int64
	droppedTotal  atomic.Int64
	wg            sync.WaitGroup
	ctx           context.Context
	cancel        context.CancelFunc
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
		redactor: NewRedactor(cfg.RedactPII),
		config:   cfg,
		queue:    make(chan *CaptureRecord, queueSize),
		ctx:      ctx,
		cancel:   cancel,
	}

	for i := 0; i < workerCount; i++ {
		cq.wg.Add(1)
		go cq.worker()
	}

	return cq
}

// ShouldCapture evaluates sampling rate and opt-in/out policies.
func (cq *CaptureQueue) ShouldCapture(virtualKey string) bool {
	if !cq.config.Enabled {
		return false
	}

	// Opt-out check
	for _, k := range cq.config.OptOutKeys {
		if k == virtualKey {
			return false
		}
	}

	// Opt-in check: if opt-in keys configured, key must match
	if len(cq.config.OptInKeys) > 0 {
		matched := false
		for _, k := range cq.config.OptInKeys {
			if k == virtualKey {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// Sampling rate check
	if cq.config.SamplingRate < 1.0 {
		if rand.Float64() > cq.config.SamplingRate {
			return false
		}
	}

	return true
}

// Enqueue asynchronously pushes a capture record into the buffer.
// Completely non-blocking: drops record and increments counter if buffer is saturated.
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
		// Ring-buffer overflow: drop record to protect gateway hot path!
		cq.droppedTotal.Add(1)
		return false
	}
}

// worker batches records and flushes to storage.
func (cq *CaptureQueue) worker() {
	defer cq.wg.Done()

	batch := make([]*CaptureRecord, 0, 64)
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	flush := func() {
		if len(batch) == 0 {
			return
		}
		// Redact PII or hash before persisting
		for _, r := range batch {
			if cq.config.StoreHashesOnly {
				r.MessagesJSON = HashString(r.MessagesJSON)
				r.ResponseJSON = HashString(r.ResponseJSON)
				r.HashOnly = true
			} else if cq.config.RedactPII {
				r.MessagesJSON = cq.redactor.Redact(r.MessagesJSON)
				r.ResponseJSON = cq.redactor.Redact(r.ResponseJSON)
			}
		}

		_ = cq.storage.SaveCapturesBatch(batch)
		batch = batch[:0]
	}

	for {
		select {
		case <-cq.ctx.Done():
			// Drain remaining items before exiting
			for {
				select {
				case rec := <-cq.queue:
					batch = append(batch, rec)
					if len(batch) >= 64 {
						flush()
					}
				default:
					flush()
					return
				}
			}
		case rec := <-cq.queue:
			batch = append(batch, rec)
			if len(batch) >= 64 {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

// Stop gracefully drains the queue and waits for workers to exit.
func (cq *CaptureQueue) Stop() {
	cq.cancel()
	cq.wg.Wait()
}

// GetMetrics returns capture and drop counts.
func (cq *CaptureQueue) GetMetrics() (int64, int64) {
	return cq.capturesTotal.Load(), cq.droppedTotal.Load()
}
