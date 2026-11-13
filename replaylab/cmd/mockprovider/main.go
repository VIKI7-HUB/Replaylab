// Command mockprovider serves a deterministic OpenAI-compatible API for
// zero-cost demos. See internal/mockprovider for model behaviour tiers.
package main

import (
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/maximhq/bifrost/plugins/replaylab/internal/mockprovider"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}
	scale := 1.0
	if v := os.Getenv("MOCK_LATENCY_SCALE"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 {
			scale = f
		}
	}

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           mockprovider.Handler(mockprovider.Options{LatencyScale: scale}),
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("Mock OpenAI-compatible provider listening on :%s (latency scale %.2f)", port, scale)
	log.Fatal(srv.ListenAndServe())
}
