package tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/maximhq/bifrost/plugins/replaylab/plugins/replaylab"
)

const okCompletion = `{"choices":[{"message":{"role":"assistant","content":"Atomic operations complete without interruption."}}],"usage":{"prompt_tokens":10,"completion_tokens":10}}`

func TestRateLimitedReplayIsRetriedAndSucceeds(t *testing.T) {
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Every other call is rate limited, the way a per-minute cap behaves.
		if calls.Add(1)%2 == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":{"message":"Rate limit reached. Please try again in 10ms."}}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(okCompletion))
	}))
	defer upstream.Close()
	lab := newLab(t, upstream.URL)
	seed(t, lab.Storage(), 3, "gpt-4o")

	exp, err := lab.Engine().StartExperiment(replaylab.CreateExperimentRequest{
		OriginalModel: "gpt-4o", CandidateModels: []string{"gpt-4o-mini"}, SampleSize: 3, BudgetCapDollars: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	final := waitTerminal(t, lab.Storage(), exp.ID)
	if final.Status != replaylab.StatusCompleted {
		t.Fatalf("status = %s, want completed", final.Status)
	}
	if got := final.Results[0]; got.FailedRequests != 0 || got.SuccessfulRequests != 3 {
		t.Fatalf("want 3 successes and no failures, got %d ok / %d failed", got.SuccessfulRequests, got.FailedRequests)
	}
	if calls.Load() != 6 {
		t.Fatalf("upstream calls = %d, want 6 (each replay retried once)", calls.Load())
	}
}

func listedModels(t *testing.T, lab *replaylab.Plugin) []string {
	t.Helper()
	rec := httptest.NewRecorder()
	lab.GetAPIHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/replaylab/models", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /replaylab/models = %d", rec.Code)
	}
	var models []struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &models); err != nil {
		t.Fatal(err)
	}
	names := make([]string, len(models))
	for i, m := range models {
		names[i] = m.Model
	}
	return names
}

func TestModelsListsOnlyWhatTheGatewayRoutes(t *testing.T) {
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"groq/openai/gpt-oss-120b"},{"id":"groq/openai/gpt-oss-20b"},{"id":"groq/whisper-large-v3"}]}`))
	}))
	defer gateway.Close()

	got := listedModels(t, newLab(t, gateway.URL))
	want := []string{"groq/openai/gpt-oss-120b", "groq/openai/gpt-oss-20b"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("models = %v, want %v", got, want)
	}
}

func TestModelsFallsBackToPriceBookWithoutAGatewayList(t *testing.T) {
	gateway := httptest.NewServer(http.NotFoundHandler())
	defer gateway.Close()

	if got := listedModels(t, newLab(t, gateway.URL)); len(got) != len(replaylab.KnownPricing) {
		t.Fatalf("listed %d models, want the whole price book (%d)", len(got), len(replaylab.KnownPricing))
	}
}

func TestProviderOutageIsRetriedAndSucceeds(t *testing.T) {
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1)%2 == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":{"message":"This model is currently experiencing high demand. Please try again in 10ms."}}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(okCompletion))
	}))
	defer upstream.Close()
	lab := newLab(t, upstream.URL)
	seed(t, lab.Storage(), 2, "gpt-4o")

	exp, err := lab.Engine().StartExperiment(replaylab.CreateExperimentRequest{
		OriginalModel: "gpt-4o", CandidateModels: []string{"gpt-4o-mini"}, SampleSize: 2, BudgetCapDollars: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	final := waitTerminal(t, lab.Storage(), exp.ID)
	if got := final.Results[0]; got.FailedRequests != 0 || got.SuccessfulRequests != 2 {
		t.Fatalf("want 2 successes and no failures, got %d ok / %d failed", got.SuccessfulRequests, got.FailedRequests)
	}
}
