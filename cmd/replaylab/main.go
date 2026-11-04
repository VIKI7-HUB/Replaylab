package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/maximhq/bifrost/plugins/replaylab/plugins/replaylab"
)

func main() {
	port := flag.String("port", "8085", "HTTP server port")
	dbPath := flag.String("db", "replaylab.db", "SQLite database path")
	gatewayURL := flag.String("gateway", "http://localhost:8081", "Target gateway or mock provider URL")
	flag.Parse()

	if envPort := os.Getenv("PORT"); envPort != "" {
		*port = envPort
	}
	if envDB := os.Getenv("DB_PATH"); envDB != "" {
		*dbPath = envDB
	}
	if envGateway := os.Getenv("GATEWAY_URL"); envGateway != "" {
		*gatewayURL = envGateway
	}

	cfg := replaylab.DefaultConfig()
	cfg.DBPath = *dbPath
	cfg.GatewayURL = *gatewayURL

	plugin, err := replaylab.NewPlugin(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize Replay Lab plugin: %v", err)
	}
	defer plugin.Cleanup()

	server := &http.Server{
		Addr:         ":" + *port,
		Handler:      plugin.GetAPIHandler(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
	}

	go func() {
		log.Printf("Bifrost Replay Lab API running on :%s (gateway: %s, db: %s)", *port, *gatewayURL, *dbPath)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server ListenAndServe error: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down Replay Lab server gracefully...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server shutdown failed: %v", err)
	}
	log.Println("Replay Lab server exited cleanly.")
}
