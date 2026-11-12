// Command bifrost_embedded runs Replay Lab as a native plugin inside the real
// Bifrost core (Go SDK), with no API keys:
//
//	go run ./demo/bifrost_embedded
//
// Traffic goes client -> Bifrost -> mock OpenAI provider. Replay Lab's
// PreLLMHook/PostLLMHook capture it; the replay engine then compares cheaper
// candidates and prints the report. The /replaylab API is served on -api
// while the demo runs.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	bifrost "github.com/maximhq/bifrost/core"
	"github.com/maximhq/bifrost/core/schemas"

	"github.com/maximhq/bifrost/plugins/replaylab/internal/mockprovider"
	"github.com/maximhq/bifrost/plugins/replaylab/plugins/replaylab"
)

// account routes the OpenAI provider to the mock server.
type account struct{ baseURL string }

func (a account) GetConfiguredProviders() ([]schemas.ModelProvider, error) {
	return []schemas.ModelProvider{schemas.OpenAI}, nil
}

func (a account) GetKeysForProvider(context.Context, schemas.ModelProvider) ([]schemas.Key, error) {
	return []schemas.Key{{ID: "demo", Value: *schemas.NewSecretVar("sk-demo"), Models: schemas.WhiteList{"*"}, Weight: 1}}, nil
}

func (a account) GetConfigForProvider(schemas.ModelProvider) (*schemas.ProviderConfig, error) {
	network := schemas.DefaultNetworkConfig
	network.BaseURL = a.baseURL
	return &schemas.ProviderConfig{NetworkConfig: network, ConcurrencyAndBufferSize: schemas.DefaultConcurrencyAndBufferSize}, nil
}

type promptTemplate struct {
	Category       string `json:"category"`
	Prompt         string `json:"prompt"`
	ExpectedFormat string `json:"expected_format"`
}

func main() {
	requests := flag.Int("requests", 120, "chat requests to send through Bifrost")
	sample := flag.Int("sample", 50, "captures to replay per candidate")
	candidates := flag.String("candidates", "gpt-4o-mini,claude-3-haiku,meta-llama-3.1-8b", "candidate models")
	apiAddr := flag.String("api", "127.0.0.1:8086", "address for the /replaylab API (empty to disable)")
	flag.Parse()

	raw, err := os.ReadFile("demo/sample_prompts.json")
	if err != nil {
		log.Fatalf("read prompts (run from the repo root): %v", err)
	}
	var templates []promptTemplate
	if err := json.Unmarshal(raw, &templates); err != nil {
		log.Fatal(err)
	}

	dir, err := os.MkdirTemp("", "replaylab-bifrost-*")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir)

	mock := httptest.NewServer(mockprovider.Handler(mockprovider.Options{LatencyScale: 0.25}))
	defer mock.Close()

	cfg := replaylab.DefaultConfig()
	cfg.DBPath = filepath.Join(dir, "replaylab.db")
	cfg.GatewayURL = mock.URL // replays go straight to the provider in this demo
	cfg.APIListenAddr = *apiAddr
	lab, err := replaylab.NewPlugin(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer lab.Cleanup()

	client, err := bifrost.Init(context.Background(), schemas.BifrostConfig{
		Account:    account{baseURL: mock.URL},
		LLMPlugins: []schemas.LLMPlugin{lab},
		Logger:     bifrost.NewNoOpLogger(),
	})
	if err != nil {
		log.Fatalf("bifrost init: %v", err)
	}
	defer client.Shutdown()

	fmt.Println("== Replay Lab running as a native Bifrost plugin ==")
	fmt.Printf("1) Sending %d gpt-4o requests through Bifrost...\n", *requests)
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	var mu sync.Mutex
	failures := 0
	for i := 0; i < *requests; i++ {
		t := templates[i%len(templates)]
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			ctx, cancel := schemas.NewBifrostContextWithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			ctx.SetValue(schemas.BifrostContextKeyVirtualKey, "vk_demo_prod")
			req := &schemas.BifrostChatRequest{
				Provider: schemas.OpenAI,
				Model:    "gpt-4o",
				Input: []schemas.ChatMessage{{
					Role:    schemas.ChatMessageRoleUser,
					Content: &schemas.ChatMessageContent{ContentStr: schemas.Ptr(t.Prompt)},
				}},
			}
			if t.ExpectedFormat == "json" {
				var format any = map[string]any{"type": "json_object"}
				req.Params = &schemas.ChatParameters{ResponseFormat: &format}
			}
			if _, berr := client.ChatCompletionRequest(ctx, req); berr != nil {
				mu.Lock()
				failures++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if n, _ := lab.Storage().GetCaptureCount(); n >= int64(*requests-failures) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	n, _ := lab.Storage().GetCaptureCount()
	fmt.Printf("   captured %d requests via PreLLMHook/PostLLMHook (%d failed)\n", n, failures)
	if *apiAddr != "" {
		fmt.Printf("   API: http://%s/replaylab/captures?limit=5\n", *apiAddr)
	}

	cands := strings.Split(*candidates, ",")
	fmt.Printf("2) Replaying %d captures against %v...\n", *sample, cands)
	exp, err := lab.Engine().StartExperiment(replaylab.CreateExperimentRequest{
		Name: "Bifrost embedded trial", OriginalModel: "gpt-4o", CandidateModels: cands, SampleSize: *sample, BudgetCapDollars: 5,
	})
	if err != nil {
		log.Fatal(err)
	}
	events, unsubscribe := lab.Engine().SubscribeEvents(exp.ID)
	defer unsubscribe()
	for ev := range events {
		fmt.Printf("\r   [%3.0f%%] %d/%d replays", ev.ProgressPercent, ev.CompletedCount, ev.TotalCount)
		if ev.Final {
			fmt.Println()
			break
		}
	}
	final, _ := lab.Storage().GetExperiment(exp.ID)
	fmt.Println("\n3) Report:")
	fmt.Println(final.SummaryMarkdown)
}
