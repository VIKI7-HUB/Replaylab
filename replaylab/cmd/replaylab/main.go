// Command replaylab runs Replay Lab as a sidecar: it serves the /replaylab
// API and an OpenAI-compatible capturing proxy at /v1/chat/completions that
// forwards to the gateway and records each exchange.
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/maximhq/bifrost/plugins/replaylab/plugins/replaylab"
)

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	port := flag.String("port", envOr("PORT", "8085"), "HTTP server port (env PORT)")
	dbPath := flag.String("db", envOr("DB_PATH", "replaylab.db"), "SQLite database path (env DB_PATH)")
	gatewayURL := flag.String("gateway", envOr("GATEWAY_URL", "http://localhost:8081"), "Upstream OpenAI-compatible gateway URL (env GATEWAY_URL)")
	flag.Parse()

	cfg := replaylab.DefaultConfig()
	cfg.DBPath = *dbPath
	cfg.GatewayURL = *gatewayURL
	cfg.ReplayAuthToken = os.Getenv("REPLAY_AUTH_TOKEN")

	plugin, err := replaylab.NewPlugin(cfg)
	if err != nil {
		log.Fatalf("failed to initialize Replay Lab: %v", err)
	}
	defer plugin.Cleanup()

	mux := http.NewServeMux()
	mux.Handle(replaylab.RoutePrefix, plugin.GetAPIHandler())
	mux.Handle("/v1/chat/completions", allowBrowsers(plugin.ProxyHandler()))

	server := &http.Server{
		Addr:              ":" + *port,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      5 * time.Minute, // proxied completions can be slow; SSE lifts this per stream
		IdleTimeout:       2 * time.Minute,
	}

	go func() {
		log.Printf("Replay Lab listening on :%s (gateway %s, db %s)", *port, *gatewayURL, *dbPath)
		log.Printf("  API:   http://localhost:%s/replaylab/health", *port)
		log.Printf("  Proxy: point OpenAI clients at http://localhost:%s/v1", *port)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v", err)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()

	log.Println("shutting down...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
}

// allowBrowsers adds the CORS headers a browser needs to call the proxy from
// another origin, as the operator console does when it sends sample traffic.
func allowBrowsers(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
