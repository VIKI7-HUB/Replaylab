// Command run_experiment is a self-contained, zero-cost Replay Lab demo.
//
// It starts an in-process mock provider and Replay Lab, sends realistic
// gpt-4o traffic through the capturing proxy (so captures are recorded the
// same way production traffic would be), then replays that traffic against
// cheaper candidates and prints the recommendation report.
//
//	go run ./demo/run_experiment
//	go run ./demo/run_experiment -requests 300 -sample 80 -candidates gpt-4o-mini,claude-3-haiku,meta-llama-3.1-8b
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/maximhq/bifrost/plugins/replaylab/internal/mockprovider"
	"github.com/maximhq/bifrost/plugins/replaylab/plugins/replaylab"
)

type promptTemplate struct {
	Category       string `json:"category"`
	Prompt         string `json:"prompt"`
	ExpectedFormat string `json:"expected_format"`
}

func main() {
	requests := flag.Int("requests", 200, "baseline requests to capture")
	sample := flag.Int("sample", 50, "captures to replay per candidate")
	budget := flag.Float64("budget", 5.0, "hard budget cap in USD")
	baseline := flag.String("baseline", "gpt-4o", "baseline model")
	candidates := flag.String("candidates", "gpt-4o-mini,claude-3-haiku,meta-llama-3.1-8b", "comma-separated candidate models")
	dbPath := flag.String("db", "", "SQLite path (default: temporary file)")
	prompts := flag.String("prompts", "demo/sample_prompts.json", "prompt templates file")
	latency := flag.Float64("latency", 0.25, "mock latency scale (1 = realistic)")
	flag.Parse()

	templates, err := loadTemplates(*prompts)
	if err != nil {
		log.Fatal(err)
	}

	if *dbPath == "" {
		dir, err := os.MkdirTemp("", "replaylab-demo-*")
		if err != nil {
			log.Fatal(err)
		}
		defer os.RemoveAll(dir)
		*dbPath = filepath.Join(dir, "demo.db")
	}

	mock := httptest.NewServer(mockprovider.Handler(mockprovider.Options{LatencyScale: *latency}))
	defer mock.Close()

	cfg := replaylab.DefaultConfig()
	cfg.DBPath = *dbPath
	cfg.GatewayURL = mock.URL
	plugin, err := replaylab.NewPlugin(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer plugin.Cleanup()

	proxy := httptest.NewServer(plugin.ProxyHandler())
	defer proxy.Close()

	banner("BIFROST REPLAY LAB: END-TO-END DEMO")
	fmt.Printf("1) Capturing %d %s requests through the proxy (PII is redacted before storage)...\n", *requests, *baseline)
	sendTraffic(proxy.URL+"/v1/chat/completions", *baseline, templates, *requests)
	waitForCaptures(plugin, *requests)

	cands := strings.Split(*candidates, ",")
	fmt.Printf("2) Replaying %d captures against %v with a $%.2f hard cap...\n", *sample, cands, *budget)
	exp, err := plugin.Engine().StartExperiment(replaylab.CreateExperimentRequest{
		Name:             "Production cost optimization trial",
		OriginalModel:    *baseline,
		CandidateModels:  cands,
		SampleSize:       *sample,
		BudgetCapDollars: *budget,
	})
	if err != nil {
		log.Fatal(err)
	}

	events, unsubscribe := plugin.Engine().SubscribeEvents(exp.ID)
	defer unsubscribe()
	for ev := range events {
		fmt.Printf("\r   [%3.0f%%] %d/%d replays | spent $%.4f | %-20s",
			ev.ProgressPercent, ev.CompletedCount, ev.TotalCount, ev.SpentDollars, ev.CandidateModel)
		if ev.Final {
			fmt.Printf("\n   %s\n", ev.Message)
			break
		}
	}

	final, err := plugin.Storage().GetExperiment(exp.ID)
	if err != nil || final == nil {
		log.Fatalf("failed to load experiment: %v", err)
	}
	fmt.Println("\n3) Report:")
	fmt.Println(final.SummaryMarkdown)
}

func loadTemplates(path string) ([]promptTemplate, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read prompts (run from the repo root): %w", err)
	}
	var t []promptTemplate
	if err := json.Unmarshal(b, &t); err != nil {
		return nil, fmt.Errorf("parse prompts: %w", err)
	}
	if len(t) == 0 {
		return nil, fmt.Errorf("no prompts in %s", path)
	}
	return t, nil
}

// sendTraffic posts n chat requests with bounded concurrency. Every fourth
// prompt carries synthetic PII to show redaction.
func sendTraffic(url, model string, templates []promptTemplate, n int) {
	var wg sync.WaitGroup
	sem := make(chan struct{}, 16)
	for i := 0; i < n; i++ {
		t := templates[i%len(templates)]
		prompt := t.Prompt
		if i%4 == 0 {
			prompt += fmt.Sprintf(" (Contact user%d@company.com, key sk-demo%024d, card 4111 1111 1111 1111)", i, i)
		}
		body := map[string]any{
			"model":    model,
			"messages": []map[string]string{{"role": "user", "content": prompt}},
		}
		if t.ExpectedFormat == "json" {
			body["response_format"] = map[string]string{"type": "json_object"}
		}
		payload, _ := json.Marshal(body)

		wg.Add(1)
		sem <- struct{}{}
		go func(tag string) {
			defer wg.Done()
			defer func() { <-sem }()
			req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set(replaylab.HeaderTag, tag)
			req.Header.Set(replaylab.HeaderVirtualKey, "vk_demo_prod")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				log.Printf("request failed: %v", err)
				return
			}
			resp.Body.Close()
		}(t.Category)
	}
	wg.Wait()
}

func waitForCaptures(p *replaylab.Plugin, want int) {
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if n, _ := p.Storage().GetCaptureCount(); n >= int64(want) {
			fmt.Printf("   %d captures stored.\n", n)
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	n, _ := p.Storage().GetCaptureCount()
	fmt.Printf("   %d captures stored (expected %d).\n", n, want)
}

func banner(s string) {
	line := strings.Repeat("=", len(s)+4)
	fmt.Printf("%s\n  %s\n%s\n", line, s, line)
}
