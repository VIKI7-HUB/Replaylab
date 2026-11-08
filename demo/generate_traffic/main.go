// Command generate_traffic sends realistic chat traffic through a running
// Replay Lab proxy so the server has captures to experiment on (useful for
// frontend development against a live API).
//
//	docker compose up -d        # or: run.bat mock + run.bat
//	go run ./demo/generate_traffic -n 200
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

type promptTemplate struct {
	Category       string `json:"category"`
	Prompt         string `json:"prompt"`
	ExpectedFormat string `json:"expected_format"`
}

func main() {
	proxy := flag.String("proxy", "http://localhost:8085", "Replay Lab base URL")
	n := flag.Int("n", 200, "number of requests")
	model := flag.String("model", "gpt-4o", "baseline model")
	prompts := flag.String("prompts", "demo/sample_prompts.json", "prompt templates file")
	concurrency := flag.Int("c", 16, "concurrent requests")
	flag.Parse()

	b, err := os.ReadFile(*prompts)
	if err != nil {
		log.Fatalf("read prompts (run from the repo root): %v", err)
	}
	var templates []promptTemplate
	if err := json.Unmarshal(b, &templates); err != nil || len(templates) == 0 {
		log.Fatalf("parse prompts: %v", err)
	}

	url := *proxy + "/v1/chat/completions"
	client := &http.Client{Timeout: 60 * time.Second}
	var ok, failed atomic.Int64
	var wg sync.WaitGroup
	sem := make(chan struct{}, max(1, *concurrency))
	start := time.Now()

	for i := 0; i < *n; i++ {
		t := templates[i%len(templates)]
		prompt := t.Prompt
		if i%4 == 0 {
			prompt += fmt.Sprintf(" (Contact user%d@company.com, key sk-demo%024d, card 4111 1111 1111 1111)", i, i)
		}
		body := map[string]any{
			"model":    *model,
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
			req.Header.Set("X-Replaylab-Tag", tag)
			req.Header.Set("X-Bf-Vk", "vk_demo_prod")
			resp, err := client.Do(req)
			if err != nil {
				failed.Add(1)
				return
			}
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				ok.Add(1)
			} else {
				failed.Add(1)
			}
		}(t.Category)
	}
	wg.Wait()

	log.Printf("sent %d requests in %s: %d ok, %d failed", *n, time.Since(start).Round(time.Millisecond), ok.Load(), failed.Load())
	if failed.Load() > 0 && ok.Load() == 0 {
		log.Fatalf("no request succeeded; is Replay Lab running at %s with a reachable gateway?", *proxy)
	}
}
