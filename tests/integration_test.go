package tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/maximhq/bifrost/plugins/replaylab/plugins/replaylab"
)

func TestEndToEndReplayFlow(t *testing.T) {
	// 1. Mock OpenAI-compatible provider
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "chatcmpl-test",
			"choices": []map[string]any{
				{
					"index": 0,
					"message": map[string]string{
						"role":    "assistant",
						"content": `{"status":"ok","result":"Candidate response text"}`,
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]int{
				"prompt_tokens":     25,
				"completion_tokens": 15,
				"total_tokens":      40,
			},
		})
	}))
	defer mockServer.Close()

	// 2. Initialize Replay Lab
	tempDB := filepath.Join(t.TempDir(), "integration.db")
	cfg := replaylab.DefaultConfig()
	cfg.DBPath = tempDB
	cfg.GatewayURL = mockServer.URL
	cfg.WorkerCount = 2

	plugin, err := replaylab.NewPlugin(cfg)
	if err != nil {
		t.Fatalf("Failed to initialize plugin: %v", err)
	}
	defer plugin.Cleanup()

	// 3. Populate sample traffic
	for i := 1; i <= 5; i++ {
		reqID := "req-test-" + string(rune('a'+i))
		plugin.RecordRequest(reqID, "vk_test", "acme", "openai", "gpt-4o", `[{"role":"user","content":"Sample test prompt"}]`, "general")
		plugin.RecordResponse(reqID, `{"status":"ok","result":"Original baseline text"}`, 25, 20, 200, "", false)
	}

	// Allow workers to flush queue
	// Wait for the background writer instead of assuming a flush interval.
	storage := plugin.Storage()
	waitForCaptures(t, storage, 5)

	// 4. Create Replay Engine and run experiment
	engine := replaylab.NewReplayEngine(storage, mockServer.URL)
	exp, err := engine.StartExperiment(replaylab.CreateExperimentRequest{
		Name:             "Integration Test Experiment",
		OriginalModel:    "gpt-4o",
		CandidateModels:  []string{"gpt-4o-mini"},
		SampleSize:       5,
		BudgetCapDollars: 5.0,
		DryRun:           false,
	})
	if err != nil {
		t.Fatalf("StartExperiment failed: %v", err)
	}

	// Wait for async completion
	var completedExp *replaylab.Experiment
	for attempt := 0; attempt < 30; attempt++ {
		time.Sleep(100 * time.Millisecond)
		completedExp, _ = storage.GetExperiment(exp.ID)
		if completedExp != nil && completedExp.Status == replaylab.StatusCompleted {
			break
		}
	}

	if completedExp == nil || completedExp.Status != replaylab.StatusCompleted {
		t.Fatalf("Experiment did not complete in time, status: %v", completedExp.Status)
	}

	if len(completedExp.Results) != 1 {
		t.Fatalf("Expected 1 candidate result, got %d", len(completedExp.Results))
	}

	res := completedExp.Results[0]
	if res.CandidateModel != "gpt-4o-mini" {
		t.Errorf("Expected candidate gpt-4o-mini, got %s", res.CandidateModel)
	}
	if res.SuccessfulRequests != 5 {
		t.Errorf("Expected 5 successful replays, got %d", res.SuccessfulRequests)
	}
	if res.AvgQualityScore <= 0 {
		t.Errorf("Expected positive quality score, got %.1f", res.AvgQualityScore)
	}
	if completedExp.SummaryMarkdown == "" {
		t.Errorf("Expected generated markdown report")
	}

	// 5. Test API interception
	intercepted, code, _, body := plugin.InterceptHTTPRequest("GET", "/replaylab/health", nil, nil)
	if !intercepted || code != 200 {
		t.Errorf("Expected HTTP interception for /replaylab/health, got intercepted=%v, code=%d", intercepted, code)
	}
	if !json.Valid(body) {
		t.Errorf("Expected valid JSON from intercepted endpoint: %s", string(body))
	}
}
