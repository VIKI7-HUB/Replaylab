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

	record := &replaylab.CaptureRecord{
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

	for i := 0; i < b.N; i++ {
		queue.Enqueue(record)
	}
}
