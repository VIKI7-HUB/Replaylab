package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/maximhq/bifrost/plugins/replaylab/plugins/replaylab"
)

type PromptTemplate struct {
	Category       string `json:"category"`
	Prompt         string `json:"prompt"`
	ExpectedFormat string `json:"expected_format"`
}

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

	cfg := replaylab.DefaultConfig()
	cfg.RedactPII = true
	redactor := replaylab.NewRedactor(true)

	// Load templates
	templatesBytes, err := os.ReadFile("demo/sample_prompts.json")
	if err != nil {
		templatesBytes, err = os.ReadFile("sample_prompts.json")
		if err != nil {
			log.Fatalf("Failed to read sample_prompts.json: %v", err)
		}
	}

	var templates []PromptTemplate
	if err := json.Unmarshal(templatesBytes, &templates); err != nil {
		log.Fatalf("Failed to unmarshal templates: %v", err)
	}

	totalTraffic := 200
	log.Printf("Generating %d varied production requests with baseline model 'gpt-4o' into %s...", totalTraffic, dbPath)

	records := make([]*replaylab.CaptureRecord, 0, totalTraffic)
	startTime := time.Now().Add(-2 * time.Hour)

	tenants := []string{"tenant-acme", "tenant-stripe", "tenant-fintech", "tenant-startup"}
	virtualKeys := []string{"vk_live_enterprise_01", "vk_live_prod_02", "vk_live_batch_03"}

	for i := 0; i < totalTraffic; i++ {
		t := templates[i%len(templates)]
		tenant := tenants[rand.Intn(len(tenants))]
		vk := virtualKeys[rand.Intn(len(virtualKeys))]

		// Introduce synthetic PII in ~25% of requests to test redactor
		promptText := t.Prompt
		if i%4 == 0 {
			promptText = fmt.Sprintf("%s (Contact user%d@company.com with secret api key sk-%s and card 4111222233334444)", promptText, i, uuid.NewString()[:24])
		}

		// Responses
		var responseText string
		if t.ExpectedFormat == "json" {
			responseText = fmt.Sprintf(`{"status":"ok","id":%d,"category":"%s","data":"Verified structured extraction completed"}`, i+100, t.Category)
		} else {
			responseText = fmt.Sprintf("High fidelity analytical result for request #%d in category %s.", i+1, t.Category)
		}

		promptTokens := len(promptText)/4 + 20
		compTokens := len(responseText)/4 + 30
		latency := int64(140 + rand.Intn(80))
		cost := replaylab.EstimateCost("gpt-4o", promptTokens, compTokens)

		record := &replaylab.CaptureRecord{
			ID:               fmt.Sprintf("cap-%s", uuid.NewString()[:10]),
			RequestID:        fmt.Sprintf("req-%s", uuid.NewString()[:12]),
			Timestamp:        startTime.Add(time.Duration(i*30) * time.Second),
			VirtualKey:       vk,
			Tenant:           tenant,
			Provider:         "openai",
			Model:            "gpt-4o",
			MessagesJSON:     redactor.Redact(promptText),
			ResponseJSON:     responseText,
			PromptTokens:     promptTokens,
			CompletionTokens: compTokens,
			TotalTokens:      promptTokens + compTokens,
			LatencyMs:        latency,
			CostEstimate:     cost,
			StatusCode:       200,
			Tag:              t.Category,
		}

		records = append(records, record)
	}

	if err := storage.SaveCapturesBatch(records); err != nil {
		log.Fatalf("Failed to save captures batch: %v", err)
	}

	count, _ := storage.GetCaptureCount()
	log.Printf("Successfully generated and stored %d capture records! Total database records: %d", totalTraffic, count)
}
