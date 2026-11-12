package tests

// These tests run the real Bifrost core (github.com/maximhq/bifrost/core)
// with Replay Lab registered as an LLM plugin, routed to the mock provider.

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	bifrost "github.com/maximhq/bifrost/core"
	"github.com/maximhq/bifrost/core/schemas"

	"github.com/maximhq/bifrost/plugins/replaylab/internal/mockprovider"
	"github.com/maximhq/bifrost/plugins/replaylab/plugins/replaylab"
)

// mockAccount configures a single OpenAI provider pointed at baseURL.
type mockAccount struct{ baseURL string }

func (a mockAccount) GetConfiguredProviders() ([]schemas.ModelProvider, error) {
	return []schemas.ModelProvider{schemas.OpenAI}, nil
}

func (a mockAccount) GetKeysForProvider(context.Context, schemas.ModelProvider) ([]schemas.Key, error) {
	return []schemas.Key{{ID: "test-key", Value: *schemas.NewSecretVar("sk-test"), Models: schemas.WhiteList{"*"}, Weight: 1}}, nil
}

func (a mockAccount) GetConfigForProvider(schemas.ModelProvider) (*schemas.ProviderConfig, error) {
	network := schemas.DefaultNetworkConfig
	network.BaseURL = a.baseURL
	network.MaxRetries = 0
	return &schemas.ProviderConfig{
		NetworkConfig:            network,
		ConcurrencyAndBufferSize: schemas.DefaultConcurrencyAndBufferSize,
	}, nil
}

func startBifrost(t *testing.T, upstream string, lab *replaylab.Plugin) *bifrost.Bifrost {
	t.Helper()
	client, err := bifrost.Init(context.Background(), schemas.BifrostConfig{
		Account:    mockAccount{baseURL: upstream},
		LLMPlugins: []schemas.LLMPlugin{lab},
		Logger:     bifrost.NewNoOpLogger(),
	})
	if err != nil {
		t.Fatalf("bifrost.Init: %v", err)
	}
	t.Cleanup(client.Shutdown)
	return client
}

func chat(t *testing.T, client *bifrost.Bifrost, ctx *schemas.BifrostContext, model, prompt string) *schemas.BifrostError {
	t.Helper()
	_, berr := client.ChatCompletionRequest(ctx, &schemas.BifrostChatRequest{
		Provider: schemas.OpenAI,
		Model:    model,
		Input: []schemas.ChatMessage{{
			Role:    schemas.ChatMessageRoleUser,
			Content: &schemas.ChatMessageContent{ContentStr: schemas.Ptr(prompt)},
		}},
		Params: &schemas.ChatParameters{MaxCompletionTokens: schemas.Ptr(64)},
	})
	return berr
}

func waitForCaptures(t *testing.T, s replaylab.Storage, want int64) []*replaylab.CaptureRecord {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if n, _ := s.GetCaptureCount(); n >= want {
			caps, _ := s.GetCaptures(replaylab.CaptureFilter{Limit: 100})
			return caps
		}
		time.Sleep(25 * time.Millisecond)
	}
	n, _ := s.GetCaptureCount()
	t.Fatalf("expected %d captures, have %d", want, n)
	return nil
}

func TestBifrostPluginCapturesChatTraffic(t *testing.T) {
	mock := httptest.NewServer(mockprovider.Handler(mockprovider.Options{}))
	defer mock.Close()
	lab := newLab(t, mock.URL)
	client := startBifrost(t, mock.URL, lab)

	ctx, cancel := schemas.NewBifrostContextWithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ctx.SetValue(schemas.BifrostContextKeyVirtualKey, "vk_team_a")
	if berr := chat(t, client, ctx, "gpt-4o", "Email alice@example.com a summary of atomic operations"); berr != nil {
		t.Fatalf("chat failed: %+v", berr.Error)
	}

	c := waitForCaptures(t, lab.Storage(), 1)[0]
	if c.Model != "gpt-4o" || c.Provider != "openai" {
		t.Errorf("model/provider = %s/%s", c.Model, c.Provider)
	}
	if c.VirtualKey != "vk_team_a" {
		t.Errorf("virtual key = %q", c.VirtualKey)
	}
	if strings.Contains(c.MessagesJSON, "alice@example.com") || !strings.Contains(c.MessagesJSON, "[REDACTED_EMAIL]") {
		t.Errorf("messages not redacted: %s", c.MessagesJSON)
	}
	if !strings.Contains(c.ParamsJSON, "max_completion_tokens") {
		t.Errorf("params not captured: %q", c.ParamsJSON)
	}
	if c.ResponseJSON == "" || c.PromptTokens == 0 || c.StatusCode != 200 {
		t.Errorf("response not captured: status=%d tokens=%d content=%q", c.StatusCode, c.PromptTokens, c.ResponseJSON)
	}
}

func TestBifrostPluginRecordsProviderErrors(t *testing.T) {
	mock := httptest.NewServer(mockprovider.Handler(mockprovider.Options{}))
	defer mock.Close()
	lab := newLab(t, mock.URL)
	client := startBifrost(t, mock.URL, lab)

	// The mock returns 503 for some prompts on small models; find one.
	var failed bool
	for i := 0; i < 60 && !failed; i++ {
		ctx, cancel := schemas.NewBifrostContextWithTimeout(context.Background(), 10*time.Second)
		failed = chat(t, client, ctx, "meta-llama-3.1-8b", "prompt number "+strings.Repeat("x", i)) != nil
		cancel()
	}
	if !failed {
		t.Skip("mock produced no error in 60 attempts")
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		caps, _ := lab.Storage().GetCaptures(replaylab.CaptureFilter{Limit: 100})
		for _, c := range caps {
			if c.StatusCode >= 400 && c.ErrorMessage != "" {
				return
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("provider error was not recorded")
}

func TestBifrostPluginSkipsReplayTraffic(t *testing.T) {
	mock := httptest.NewServer(mockprovider.Handler(mockprovider.Options{}))
	defer mock.Close()
	lab := newLab(t, mock.URL)
	client := startBifrost(t, mock.URL, lab)

	ctx, cancel := schemas.NewBifrostContextWithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// bifrost-http runs the transport pre-hook first; it marks replays from the header.
	if _, err := lab.HTTPTransportPreHook(ctx, &schemas.HTTPRequest{
		Method: "POST", Path: "/v1/chat/completions",
		Headers: map[string]string{"x-replaylab-replay": "1"},
	}); err != nil {
		t.Fatal(err)
	}
	if berr := chat(t, client, ctx, "gpt-4o", "replayed prompt"); berr != nil {
		t.Fatalf("chat failed: %+v", berr.Error)
	}
	time.Sleep(400 * time.Millisecond)
	if n, _ := lab.Storage().GetCaptureCount(); n != 0 {
		t.Fatalf("replay was captured: %d", n)
	}
}

func TestBifrostTransportHookServesAPI(t *testing.T) {
	lab := newLab(t, "http://127.0.0.1:1")
	ctx, cancel := schemas.NewBifrostContextWithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := lab.HTTPTransportPreHook(ctx, &schemas.HTTPRequest{
		Method: "GET", Path: "/replaylab/captures", Query: map[string]string{"limit": "5"},
	})
	if err != nil || resp == nil || resp.StatusCode != 200 || string(resp.Body) != "[]\n" {
		t.Fatalf("unexpected API response: %+v err=%v", resp, err)
	}

	// Ordinary gateway traffic passes through untouched.
	resp, err = lab.HTTPTransportPreHook(ctx, &schemas.HTTPRequest{Method: "POST", Path: "/v1/chat/completions"})
	if resp != nil || err != nil {
		t.Fatalf("gateway request was intercepted: %+v %v", resp, err)
	}
}

func TestEndToEndCaptureThroughBifrostThenReplay(t *testing.T) {
	// Simulated latency keeps the latency comparison meaningful: with none,
	// every call takes 0-2 ms of scheduler noise and a 1 ms wobble reads as a
	// >40% regression, flipping the verdict to SWITCH_WITH_CAUTION.
	mock := httptest.NewServer(mockprovider.Handler(mockprovider.Options{LatencyScale: 0.25}))
	defer mock.Close()
	lab := newLab(t, mock.URL)
	client := startBifrost(t, mock.URL, lab)

	prompts := []string{
		"Extract the structured JSON object containing name, email, and company from: 'Bo, bo@x.io, Acme'",
		"Explain why atomic CPU operations avoid race conditions in concurrent code.",
		"Summarize the advantages of micro-frontends versus monolithic single page apps.",
	}
	for i := 0; i < 24; i++ {
		ctx, cancel := schemas.NewBifrostContextWithTimeout(context.Background(), 10*time.Second)
		if berr := chat(t, client, ctx, "gpt-4o", prompts[i%len(prompts)]); berr != nil {
			cancel()
			t.Fatalf("chat %d failed: %+v", i, berr.Error)
		}
		cancel()
	}
	waitForCaptures(t, lab.Storage(), 24)

	exp, err := lab.Engine().StartExperiment(replaylab.CreateExperimentRequest{
		OriginalModel: "gpt-4o", CandidateModels: []string{"gpt-4o-mini", "meta-llama-3.1-8b"}, SampleSize: 24,
	})
	if err != nil {
		t.Fatal(err)
	}
	final := waitTerminal(t, lab.Storage(), exp.ID)
	if final.Status != replaylab.StatusCompleted || len(final.Recommendations) != 2 {
		t.Fatalf("status=%s recs=%d", final.Status, len(final.Recommendations))
	}
	byModel := map[string]string{}
	for _, r := range final.Recommendations {
		byModel[r.CandidateModel] = r.Verdict
	}
	if byModel["gpt-4o-mini"] != replaylab.VerdictSafeToSwitch {
		t.Errorf("gpt-4o-mini verdict = %s, want SAFE_TO_SWITCH", byModel["gpt-4o-mini"])
	}
	if byModel["meta-llama-3.1-8b"] != replaylab.VerdictDoNotSwitch {
		t.Errorf("llama-8b verdict = %s, want DO_NOT_SWITCH", byModel["meta-llama-3.1-8b"])
	}
}
