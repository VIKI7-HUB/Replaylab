package tests

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/maximhq/bifrost/plugins/replaylab/plugins/replaylab"
)

func BenchmarkCaptureQueueEnqueue(b *testing.B) {
	tempDB := filepath.Join(b.TempDir(), "bench.db")
	storage, err := replaylab.NewSQLiteStorage(tempDB)
	if err != nil {
		b.Fatalf("failed to open storage: %v", err)
	}
	defer storage.Close()

	cfg := replaylab.DefaultConfig()
	cfg.MaxQueueSize = 500000
	cfg.WorkerCount = 4
	queue := replaylab.NewCaptureQueue(cfg, storage)
	defer queue.Stop()

	template := replaylab.CaptureRecord{
		RequestID:        "bench-req-12345",
		Timestamp:        time.Now(),
		Provider:         "openai",
		Model:            "gpt-4o",
		MessagesJSON:     `[{"role":"user","content":"Hello world benchmark prompt"}]`,
		ResponseJSON:     `{"status":"ok"}`,
		PromptTokens:     15,
		CompletionTokens: 5,
		LatencyMs:        120,
		CostEstimate:     0.0001,
		StatusCode:       200,
	}

	b.ResetTimer()
	b.ReportAllocs()

	// Each iteration enqueues a fresh record, as the gateway hot path does;
	// reusing one pointer would collide on IDs and race with the writers.
	for i := 0; i < b.N; i++ {
		rec := template
		queue.Enqueue(&rec)
	}
}

// nopStorage discards writes so the benchmark below measures only the
// hot-path cost of Enqueue, not SQLite writers competing for CPU.
type nopStorage struct{}

func (nopStorage) SaveCapture(*replaylab.CaptureRecord) error         { return nil }
func (nopStorage) SaveCapturesBatch([]*replaylab.CaptureRecord) error { return nil }
func (nopStorage) GetCaptures(replaylab.CaptureFilter) ([]*replaylab.CaptureRecord, error) {
	return nil, nil
}
func (nopStorage) GetCaptureByID(string) (*replaylab.CaptureRecord, error) { return nil, nil }
func (nopStorage) GetCaptureCount() (int64, error)                         { return 0, nil }
func (nopStorage) GetTrafficVolume(string) (replaylab.TrafficVolume, error) {
	return replaylab.TrafficVolume{}, nil
}
func (nopStorage) SaveExperiment(*replaylab.Experiment) error                { return nil }
func (nopStorage) GetExperiment(string) (*replaylab.Experiment, error)       { return nil, nil }
func (nopStorage) ListExperiments(int, int) ([]*replaylab.Experiment, error) { return nil, nil }
func (nopStorage) PurgeExpiredCaptures(int) (int64, error)                   { return 0, nil }
func (nopStorage) Close() error                                              { return nil }

func BenchmarkCaptureHotPath(b *testing.B) {
	cfg := replaylab.DefaultConfig()
	cfg.MaxQueueSize = 1 << 20
	queue := replaylab.NewCaptureQueue(cfg, nopStorage{})
	defer queue.Stop()

	template := replaylab.CaptureRecord{
		RequestID:    "bench-req",
		Timestamp:    time.Now(),
		Provider:     "openai",
		Model:        "gpt-4o",
		MessagesJSON: `[{"role":"user","content":"Hello world benchmark prompt"}]`,
		StatusCode:   200,
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec := template
		queue.Enqueue(&rec)
	}
}
