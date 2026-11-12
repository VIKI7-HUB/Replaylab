package replaylab

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"
)

const PluginName = "replaylab"

const (
	// pendingTTL bounds how long a request may wait for its response before
	// its pending context is discarded (e.g. the client disconnected).
	pendingTTL      = 10 * time.Minute
	janitorInterval = time.Minute
	purgeInterval   = time.Hour
)

// PendingCaptureContext stashes pre-request context for post-response recording.
type PendingCaptureContext struct {
	StartTime    time.Time
	RequestID    string
	VirtualKey   string
	Tenant       string
	Provider     string
	Model        string
	MessagesJSON string
	ParamsJSON   string
	Tag          string

	stream *streamCapture // set for streamed captures
}

// Plugin wires capture, replay and the REST API together. Inside Bifrost it
// is registered as a native LLM and HTTP transport plugin (see bifrost.go);
// standalone, cmd/replaylab serves the API plus a capturing proxy.
type Plugin struct {
	config     Config
	storage    Storage
	queue      *CaptureQueue
	engine     *ReplayEngine
	api        *APIHandler
	pendingMap sync.Map // requestID -> *PendingCaptureContext

	apiServer *http.Server // set when Config.APIListenAddr is used

	stop     chan struct{}
	stopOnce sync.Once
	bgDone   sync.WaitGroup
}

// NewPlugin initializes storage, the capture queue, the replay engine, the API
// handler and the background janitor.
func NewPlugin(cfg Config) (*Plugin, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid replaylab config: %w", err)
	}
	storage, err := NewSQLiteStorage(cfg.DBPath)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize replaylab storage: %w", err)
	}
	if n, err := storage.FailInterruptedExperiments(time.Now()); err != nil {
		storage.Close()
		return nil, fmt.Errorf("failed to close out interrupted experiments: %w", err)
	} else if n > 0 {
		log.Printf("replaylab: marked %d experiment(s) interrupted by the last shutdown as failed", n)
	}

	queue := NewCaptureQueue(cfg, storage)
	engine := NewReplayEngine(storage, cfg.GatewayURL)
	engine.SetAuthToken(cfg.ReplayAuthToken)
	api := NewAPIHandler(storage, engine, queue)

	p := &Plugin{
		config:  cfg,
		storage: storage,
		queue:   queue,
		engine:  engine,
		api:     api,
		stop:    make(chan struct{}),
	}
	if cfg.APIListenAddr != "" {
		if err := p.serveAPI(cfg.APIListenAddr); err != nil {
			_ = p.Cleanup()
			return nil, err
		}
	}
	p.bgDone.Add(1)
	go p.janitor()
	return p, nil
}

// serveAPI starts a dedicated listener for the /replaylab API. Embedded in
// Bifrost this is the way to get live SSE progress, since transport hooks
// buffer responses.
func (p *Plugin) serveAPI(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("replaylab API listener on %s: %w", addr, err)
	}
	p.apiServer = &http.Server{Handler: p.api, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		if err := p.apiServer.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("replaylab: API server stopped: %v", err)
		}
	}()
	log.Printf("replaylab: API listening on %s", ln.Addr())
	return nil
}

// GetName returns the canonical plugin name.
func (p *Plugin) GetName() string {
	return PluginName
}

// Cleanup stops background work, drains the capture queue and closes storage.
// It is safe to call more than once.
func (p *Plugin) Cleanup() error {
	var err error
	p.stopOnce.Do(func() {
		if p.apiServer != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = p.apiServer.Shutdown(ctx)
			cancel()
		}
		close(p.stop)
		p.bgDone.Wait()
		p.queue.Stop()
		err = p.storage.Close()
	})
	return err
}

// GetAPIHandler returns the HTTP handler for sidecar or embedded serving.
func (p *Plugin) GetAPIHandler() *APIHandler {
	return p.api
}

// Storage exposes the capture and experiment store.
func (p *Plugin) Storage() Storage {
	return p.storage
}

// Engine exposes the replay engine (used by demos and embedding code).
func (p *Plugin) Engine() *ReplayEngine {
	return p.engine
}

// RecordRequest stashes request details until the matching response arrives.
func (p *Plugin) RecordRequest(reqID, virtualKey, tenant, provider, model, messagesJSON, tag string) {
	p.RecordRequestWithParams(reqID, virtualKey, tenant, provider, model, messagesJSON, "", tag)
}

// RecordRequestWithParams is RecordRequest plus the non-message request
// parameters, which replays reuse so candidates see the same call shape.
func (p *Plugin) RecordRequestWithParams(reqID, virtualKey, tenant, provider, model, messagesJSON, paramsJSON, tag string) {
	if reqID == "" || !p.queue.ShouldCapture(virtualKey) {
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
		ParamsJSON:   paramsJSON,
		Tag:          tag,
	})
}

// RecordResponse pairs a response with its stashed request and enqueues the
// capture for the background writers. responseContent is the assistant text.
func (p *Plugin) RecordResponse(reqID string, responseContent string, promptTokens, completionTokens int, statusCode int, errMsg string, isStreaming bool) {
	val, ok := p.pendingMap.LoadAndDelete(reqID)
	if !ok {
		return
	}
	pending := val.(*PendingCaptureContext)

	p.queue.Enqueue(&CaptureRecord{
		RequestID:        pending.RequestID,
		Timestamp:        pending.StartTime,
		VirtualKey:       pending.VirtualKey,
		Tenant:           pending.Tenant,
		Provider:         pending.Provider,
		Model:            pending.Model,
		MessagesJSON:     pending.MessagesJSON,
		ParamsJSON:       pending.ParamsJSON,
		ResponseJSON:     responseContent,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      promptTokens + completionTokens,
		LatencyMs:        time.Since(pending.StartTime).Milliseconds(),
		CostEstimate:     EstimateCost(captureModelForPricing(pending.Provider, pending.Model), promptTokens, completionTokens),
		StatusCode:       statusCode,
		ErrorMessage:     errMsg,
		IsStreaming:      isStreaming,
		Tag:              pending.Tag,
		CompletionState:  CompletionStateComplete,
	})
}

// InterceptHTTPRequest serves /replaylab/* requests in embedded mode. It
// returns handled=false for any other path. SSE endpoints need a streaming
// transport and should be served through GetAPIHandler instead.
func (p *Plugin) InterceptHTTPRequest(method, path string, headers map[string]string, body []byte) (bool, int, map[string]string, []byte) {
	if !IsAPIPath(path) {
		return false, 0, nil, nil
	}

	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	rec := httptest.NewRecorder()
	p.api.ServeHTTP(rec, req)

	respHeaders := make(map[string]string, len(rec.Header()))
	for k := range rec.Header() {
		respHeaders[k] = rec.Header().Get(k)
	}
	return true, rec.Code, respHeaders, rec.Body.Bytes()
}

// janitor evicts abandoned pending captures and enforces retention.
func (p *Plugin) janitor() {
	defer p.bgDone.Done()
	sweep := time.NewTicker(janitorInterval)
	defer sweep.Stop()
	purge := time.NewTicker(purgeInterval)
	defer purge.Stop()

	p.purgeExpired()
	for {
		select {
		case <-p.stop:
			return
		case <-sweep.C:
			p.evictStalePending(time.Now().Add(-pendingTTL))
		case <-purge.C:
			p.purgeExpired()
		}
	}
}

// evictStalePending drops requests that never got a response. A stream that
// never ended is stored as abandoned rather than dropped.
func (p *Plugin) evictStalePending(cutoff time.Time) {
	p.pendingMap.Range(func(key, val any) bool {
		pending := val.(*PendingCaptureContext)
		if !pending.StartTime.Before(cutoff) {
			return true
		}
		if pending.stream != nil {
			p.FinishStream(key.(string), StreamOutcome{
				State:        CompletionStateAbandoned,
				ErrorMessage: "stream never ended",
			})
		} else {
			p.pendingMap.Delete(key)
		}
		return true
	})
}

func (p *Plugin) purgeExpired() {
	days := p.queue.Config().RetentionDays
	if days <= 0 {
		return
	}
	n, err := p.storage.PurgeExpiredCaptures(days)
	if err != nil {
		log.Printf("replaylab: retention purge failed: %v", err)
		return
	}
	if n > 0 {
		log.Printf("replaylab: purged %d captures older than %d days", n, days)
	}
}
