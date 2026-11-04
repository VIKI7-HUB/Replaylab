package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/maximhq/bifrost/plugins/replaylab/plugins/replaylab"
)

func main() {
	dbPath := "replaylab.db"
	if len(os.Args) > 1 {
		dbPath = os.Args[1]
	}

	storage, err := replaylab.NewSQLiteStorage(dbPath)
	if err != nil {
		log.Fatalf("Failed to open SQLite database: %v", err)
	}
	defer storage.Close()

	// Verify we have captures
	count, err := storage.GetCaptureCount()
	if err != nil || count == 0 {
		log.Printf("No captures found in %s. Please run generate_traffic first!", dbPath)
		return
	}

	// Spin up in-process mock provider on :8089 if not running
	mockAddr := "http://localhost:8089"
	go startMockServer(":8089")
	time.Sleep(100 * time.Millisecond)

	log.Println("===================================================================")
	log.Println("           BIFROST REPLAY LAB: MODEL COMPARISON RUNNER             ")
	log.Println("===================================================================")
	log.Printf("Loaded %d recorded production requests from %s", count, dbPath)

	engine := replaylab.NewReplayEngine(storage, mockAddr)

	req := replaylab.CreateExperimentRequest{
		Name:             "Production Cost Optimization Trial",
		OriginalModel:    "gpt-4o",
		CandidateModels:  []string{"gpt-4o-mini", "claude-3-haiku"},
		SampleSize:       50,
		BudgetCapDollars: 25.0,
		DryRun:           false,
	}

	log.Printf("Initiating Replay Experiment comparing '%s' against %v (sample size: %d, budget cap: $%.2f)...",
		req.OriginalModel, req.CandidateModels, req.SampleSize, req.BudgetCapDollars)

	exp, err := engine.StartExperiment(req)
	if err != nil {
		log.Fatalf("Failed to launch experiment: %v", err)
	}

	log.Printf("Experiment launched with ID: %s. Replaying live traffic...\n", exp.ID)

	// Stream progress events
	eventCh := engine.SubscribeEvents(exp.ID)
	done := make(chan struct{})

	go func() {
		for ev := range eventCh {
			fmt.Printf("\r[PROGRESS %3.0f%%] Replayed: %d/%d | Spent: $%.4f | Avg Quality: %.1f/100 | Target: %-15s",
				ev.ProgressPercent, ev.CompletedCount, ev.TotalCount, ev.SpentDollars, ev.CurrentQuality, ev.CandidateModel)
			if ev.CompletedCount >= ev.TotalCount && ev.TotalCount > 0 {
				fmt.Println()
				close(done)
				return
			}
		}
	}()

	select {
	case <-done:
		log.Println("\nAll replays completed!")
	case <-time.After(30 * time.Second):
		log.Println("\nTimeout waiting for replays.")
	}

	// Fetch final completed experiment report
	completedExp, err := storage.GetExperiment(exp.ID)
	if err != nil || completedExp == nil {
		log.Fatalf("Failed to retrieve final experiment: %v", err)
	}

	fmt.Println("\n" + completedExp.SummaryMarkdown)
}

func startMockServer(addr string) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id": "chatcmpl-mock",
			"object": "chat.completion",
			"choices": [{"index": 0, "message": {"role": "assistant", "content": "{\"status\":\"ok\",\"result\":\"High fidelity structured completion\"}"}, "finish_reason": "stop"}],
			"usage": {"prompt_tokens": 45, "completion_tokens": 30, "total_tokens": 75}
		}`))
	})
	_ = http.ListenAndServe(addr, mux)
}
