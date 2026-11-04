package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"
)

type ChatCompletionRequest struct {
	Model       string `json:"model"`
	Messages    []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
	Temperature float64 `json:"temperature"`
}

type ChatCompletionResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index        int `json:"index"`
		Message      struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}

	http.HandleFunc("/v1/chat/completions", handleChatCompletions)
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"healthy","service":"bifrost-mockprovider"}`))
	})

	log.Printf("Bifrost Mock OpenAI Provider running on :%s ...", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ChatCompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid payload: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Extract prompt snippet
	lastMsg := ""
	if len(req.Messages) > 0 {
		lastMsg = req.Messages[len(req.Messages)-1].Content
	}

	// Model-specific behavioral simulation:
	// - gpt-4o: rich high fidelity
	// - gpt-4o-mini: faster, slightly concise
	// - claude-3-haiku: super fast, concise
	// - local-small: slightly lower fidelity
	var responseContent string
	var latencyMs int

	switch {
	case strings.Contains(req.Model, "mini"):
		latencyMs = 80 + rand.Intn(40)
		if strings.Contains(strings.ToLower(lastMsg), "json") {
			responseContent = `{"status":"ok","model":"` + req.Model + `","result":"Processed JSON payload with high fidelity"}`
		} else {
			responseContent = fmt.Sprintf("Response from %s: Processed input with high efficiency.", req.Model)
		}
	case strings.Contains(req.Model, "haiku"):
		latencyMs = 50 + rand.Intn(30)
		if strings.Contains(strings.ToLower(lastMsg), "json") {
			responseContent = `{"status":"ok","model":"` + req.Model + `","result":"Processed JSON payload"}`
		} else {
			responseContent = fmt.Sprintf("Concise answer from %s: Complete summary.", req.Model)
		}
	case strings.Contains(req.Model, "gpt-4o"):
		latencyMs = 150 + rand.Intn(60)
		if strings.Contains(strings.ToLower(lastMsg), "json") {
			responseContent = `{"status":"ok","model":"` + req.Model + `","result":"Processed JSON payload with full schema fidelity and nested attributes"}`
		} else {
			responseContent = fmt.Sprintf("Comprehensive analytical answer from %s answering your question thoroughly.", req.Model)
		}
	default:
		latencyMs = 90 + rand.Intn(50)
		responseContent = fmt.Sprintf("Standard completion from %s.", req.Model)
	}

	time.Sleep(time.Duration(latencyMs) * time.Millisecond)

	promptTokens := len(lastMsg)/4 + 10
	completionTokens := len(responseContent)/4 + 15

	resp := ChatCompletionResponse{
		ID:      fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano()),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   req.Model,
		Choices: []struct {
			Index        int `json:"index"`
			Message      struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		}{
			{
				Index: 0,
				Message: struct {
					Role    string `json:"role"`
					Content string `json:"content"`
				}{
					Role:    "assistant",
					Content: responseContent,
				},
				FinishReason: "stop",
			},
		},
		Usage: struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		}{
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			TotalTokens:      promptTokens + completionTokens,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}
