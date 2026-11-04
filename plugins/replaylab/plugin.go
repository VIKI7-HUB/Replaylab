package replaylab

import (
	"bytes"
	"fmt"
	"net/http/httptest"
	"strings"
	"sync"
	"time"
)

const PluginName = "replaylab"

// PendingCaptureContext stashes pre-request context for post-response recording.
type PendingCaptureContext struct {
	StartTime    time.Time
	RequestID    string
	VirtualKey   string
	Tenant       string
	Provider     string
	Model        string
	MessagesJSON string
	Tag          string
}

// Plugin implements Bifrost Replay Lab capture, replay engine, and REST API.
type Plugin struct {
	config     Config
	storage    Storage
	queue      *CaptureQueue
	engine     *ReplayEngine
	api        *APIHandler
	pendingMap sync.Map // requestID -> *PendingCaptureContext
}

// NewPlugin initializes Replay Lab storage, capture queue, replay engine, and API handler.
func NewPlugin(cfg Config) (*Plugin, error) {
	storage, err := NewSQLiteStorage(cfg.DBPath)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize replaylab storage: %w", err)
	}

	queue := NewCaptureQueue(cfg, storage)
	engine := NewReplayEngine(storage, cfg.GatewayURL)
	api := NewAPIHandler(storage, engine, queue, cfg)

	return &Plugin{
		config:  cfg,
		storage: storage,
		queue:   queue,
		engine:  engine,
		api:     api,
	}, nil
}

// GetName returns the canonical plugin name.
func (p *Plugin) GetName() string {
	return PluginName
}

// Cleanup drains write queues and releases resources on gateway shutdown.
func (p *Plugin) Cleanup() error {
	p.queue.Stop()
	return p.storage.Close()
}

// GetAPIHandler returns the HTTP handler for sidecar or embedded serving.
func (p *Plugin) GetAPIHandler() *APIHandler {
	return p.api
}

// RecordRequest captures inbound request details asynchronously before sending to provider.
func (p *Plugin) RecordRequest(reqID, virtualKey, tenant, provider, model, messagesJSON, tag string) {
	if !p.queue.ShouldCapture(virtualKey) {
		return
	}
	p.pendingMap.Store(reqID, &PendingCaptureContext{
		StartTime:    time.Now(),
		RequestID:    reqID,
		VirtualKey:   virtualKey,
		Tenant:       tenant,
		Provider:     provider,
		Model:        model,
		MessagesJSON: messagesJSON,
		Tag:          tag,
	})
}

// RecordResponse pairs response metadata with the stashed request context and enqueues to background writer.
func (p *Plugin) RecordResponse(reqID string, responseJSON string, promptTokens, completionTokens int, statusCode int, errMsg string, isStreaming bool) {
	val, ok := p.pendingMap.LoadAndDelete(reqID)
	if !ok {
		return
	}
	pending := val.(*PendingCaptureContext)

	latency := time.Since(pending.StartTime).Milliseconds()
	cost := EstimateCost(pending.Model, promptTokens, completionTokens)

	record := &CaptureRecord{
		RequestID:        pending.RequestID,
		Timestamp:        pending.StartTime,
		VirtualKey:       pending.VirtualKey,
		Tenant:           pending.Tenant,
		Provider:         pending.Provider,
		Model:            pending.Model,
		MessagesJSON:     pending.MessagesJSON,
		ResponseJSON:     responseJSON,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      promptTokens + completionTokens,
		LatencyMs:        latency,
		CostEstimate:     cost,
		StatusCode:       statusCode,
		ErrorMessage:     errMsg,
		IsStreaming:      isStreaming,
		Tag:              pending.Tag,
	}

	p.queue.Enqueue(record)
}

// InterceptHTTPRequest checks if the incoming request path targets /replaylab/* and serves it directly.
func (p *Plugin) InterceptHTTPRequest(method, path string, headers map[string]string, body []byte) (bool, int, map[string]string, []byte) {
	if !strings.HasPrefix(path, "/replaylab/") && path != "/replaylab" {
		return false, 0, nil, nil
	}

	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	rec := httptest.NewRecorder()
	p.api.ServeHTTP(rec, req)

	respHeaders := make(map[string]string)
	for k := range rec.Header() {
		respHeaders[k] = rec.Header().Get(k)
	}

	return true, rec.Code, respHeaders, rec.Body.Bytes()
}
