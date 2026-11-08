package replaylab

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Version is reported by the health endpoint.
const Version = "1.1.0"

const (
	maxJSONBody      = 1 << 20 // 1 MiB for API request bodies
	sseHeartbeat     = 15 * time.Second
	defaultListLimit = 50
)

// APIHandler serves the /replaylab REST and SSE API.
type APIHandler struct {
	storage   Storage
	engine    *ReplayEngine
	queue     *CaptureQueue
	mux       *http.ServeMux
	cfgMu     sync.Mutex // serialises config updates
	startedAt time.Time
}

// NewAPIHandler sets up routing.
func NewAPIHandler(storage Storage, engine *ReplayEngine, queue *CaptureQueue) *APIHandler {
	h := &APIHandler{
		storage:   storage,
		engine:    engine,
		queue:     queue,
		mux:       http.NewServeMux(),
		startedAt: time.Now(),
	}
	h.registerRoutes()
	return h
}

func (h *APIHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	h.mux.ServeHTTP(w, r)
}

func (h *APIHandler) registerRoutes() {
	h.mux.HandleFunc("GET /replaylab/health", h.handleHealth)
	h.mux.HandleFunc("GET /replaylab/metrics", h.handleMetrics)
	h.mux.HandleFunc("GET /replaylab/config", h.handleGetConfig)
	h.mux.HandleFunc("PUT /replaylab/config", h.handlePutConfig)
	h.mux.HandleFunc("GET /replaylab/models", h.handleModels)
	h.mux.HandleFunc("GET /replaylab/captures", h.handleListCaptures)
	h.mux.HandleFunc("GET /replaylab/captures/{id}", h.handleGetCapture)
	h.mux.HandleFunc("GET /replaylab/experiments", h.handleListExperiments)
	h.mux.HandleFunc("POST /replaylab/experiments", h.handleCreateExperiment)
	h.mux.HandleFunc("GET /replaylab/experiments/{id}", h.handleGetExperiment)
	h.mux.HandleFunc("POST /replaylab/experiments/{id}/cancel", h.handleCancelExperiment)
	h.mux.HandleFunc("GET /replaylab/experiments/{id}/report", h.handleReport)
	h.mux.HandleFunc("GET /replaylab/experiments/{id}/events", h.handleEvents)
}

func (h *APIHandler) handleHealth(w http.ResponseWriter, r *http.Request) {
	status, httpStatus := "healthy", http.StatusOK
	count, err := h.storage.GetCaptureCount()
	if err != nil {
		status, httpStatus = "degraded", http.StatusServiceUnavailable
	}
	_, dropped := h.queue.GetMetrics()
	writeJSON(w, httpStatus, map[string]any{
		"status":             status,
		"service":            "bifrost-replaylab",
		"version":            Version,
		"uptime_seconds":     int(time.Since(h.startedAt).Seconds()),
		"stored_captures":    count,
		"dropped_captures":   dropped,
		"active_experiments": h.engine.ActiveExperiments(),
		"capture_enabled":    h.queue.Config().Enabled,
	})
}

func (h *APIHandler) handleMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	_, _ = w.Write([]byte(FormatPrometheus(h.storage, h.queue, h.engine)))
}

func (h *APIHandler) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.queue.Config())
}

// handlePutConfig merges the supplied fields into the current config, so
// clients can send only what they want to change. Startup-only fields are
// ignored.
func (h *APIHandler) handlePutConfig(w http.ResponseWriter, r *http.Request) {
	h.cfgMu.Lock()
	defer h.cfgMu.Unlock()

	current := h.queue.Config()
	updated := current
	if err := decodeJSON(w, r, &updated); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	updated.DBPath = current.DBPath
	updated.GatewayURL = current.GatewayURL
	updated.MaxQueueSize = current.MaxQueueSize
	updated.WorkerCount = current.WorkerCount
	updated.ReplayAuthToken = current.ReplayAuthToken
	updated.APIListenAddr = current.APIListenAddr
	if err := updated.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	h.queue.UpdateConfig(updated)
	writeJSON(w, http.StatusOK, updated)
}

type modelPrice struct {
	Model             string  `json:"model"`
	InputPer1MTokens  float64 `json:"input_per_1m_tokens"`
	OutputPer1MTokens float64 `json:"output_per_1m_tokens"`
}

func (h *APIHandler) handleModels(w http.ResponseWriter, r *http.Request) {
	out := make([]modelPrice, 0, len(KnownPricing))
	for name, p := range KnownPricing {
		out = append(out, modelPrice{Model: name, InputPer1MTokens: p.InputPer1M, OutputPer1MTokens: p.OutputPer1M})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Model < out[j].Model })
	writeJSON(w, http.StatusOK, out)
}

func (h *APIHandler) handleListCaptures(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, offset, err := pagination(q.Get("limit"), q.Get("offset"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	captures, err := h.storage.GetCaptures(CaptureFilter{
		Model:      q.Get("model"),
		Tag:        q.Get("tag"),
		VirtualKey: q.Get("virtual_key"),
		Limit:      limit,
		Offset:     offset,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if captures == nil {
		captures = []*CaptureRecord{}
	}
	writeJSON(w, http.StatusOK, captures)
}

func (h *APIHandler) handleGetCapture(w http.ResponseWriter, r *http.Request) {
	c, err := h.storage.GetCaptureByID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if c == nil {
		writeError(w, http.StatusNotFound, "capture not found")
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (h *APIHandler) handleListExperiments(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, offset, err := pagination(q.Get("limit"), q.Get("offset"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	experiments, err := h.storage.ListExperiments(limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if experiments == nil {
		experiments = []*Experiment{}
	}
	writeJSON(w, http.StatusOK, experiments)
}

func (h *APIHandler) handleCreateExperiment(w http.ResponseWriter, r *http.Request) {
	var req CreateExperimentRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := req.Normalize(h.queue.Config().DefaultBudgetCap); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	exp, err := h.engine.StartExperiment(req)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	status := http.StatusAccepted
	if exp.DryRun {
		status = http.StatusOK
	}
	writeJSON(w, status, exp)
}

func (h *APIHandler) loadExperiment(w http.ResponseWriter, id string) (*Experiment, bool) {
	exp, err := h.storage.GetExperiment(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return nil, false
	}
	if exp == nil {
		writeError(w, http.StatusNotFound, "experiment not found")
		return nil, false
	}
	return exp, true
}

func (h *APIHandler) handleGetExperiment(w http.ResponseWriter, r *http.Request) {
	if exp, ok := h.loadExperiment(w, r.PathValue("id")); ok {
		writeJSON(w, http.StatusOK, exp)
	}
}

func (h *APIHandler) handleCancelExperiment(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	exp, ok := h.loadExperiment(w, id)
	if !ok {
		return
	}
	if IsTerminalStatus(exp.Status) {
		writeError(w, http.StatusConflict, "experiment already "+exp.Status)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"cancelled": h.engine.CancelExperiment(id), "experiment_id": id})
}

func (h *APIHandler) handleReport(w http.ResponseWriter, r *http.Request) {
	exp, ok := h.loadExperiment(w, r.PathValue("id"))
	if !ok {
		return
	}
	if r.URL.Query().Get("format") == "markdown" {
		md := exp.SummaryMarkdown
		if md == "" {
			md = FormatMarkdownReport(exp) // still running: render progress so far
		}
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		_, _ = w.Write([]byte(md))
		return
	}
	writeJSON(w, http.StatusOK, exp)
}

// handleEvents streams experiment progress as Server-Sent Events. The stream
// always ends with an event whose "final" field is true.
func (h *APIHandler) handleEvents(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	rc := http.NewResponseController(w)

	// Subscribe before reading state so a run finishing in between cannot
	// leave this client waiting forever.
	events, unsubscribe := h.engine.SubscribeEvents(id)
	defer unsubscribe()

	exp, ok := h.loadExperiment(w, id)
	if !ok {
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	_ = rc.SetWriteDeadline(time.Time{}) // long-lived stream: lift the server write timeout
	w.WriteHeader(http.StatusOK)

	send := func(ev ReplayEvent) bool {
		data, _ := json.Marshal(ev)
		if _, err := fmt.Fprintf(w, "event: progress\ndata: %s\n\n", data); err != nil {
			return false
		}
		return rc.Flush() == nil
	}

	snapshot := ReplayEvent{
		ExperimentID:   exp.ID,
		Status:         exp.Status,
		CompletedCount: exp.CompletedReplays,
		TotalCount:     exp.TotalReplays,
		SpentDollars:   exp.SpentDollars,
		Message:        "current state",
		Final:          IsTerminalStatus(exp.Status),
	}
	if exp.TotalReplays > 0 {
		snapshot.ProgressPercent = float64(exp.CompletedReplays) / float64(exp.TotalReplays) * 100
	}
	if !send(snapshot) || snapshot.Final {
		return
	}

	heartbeat := time.NewTicker(sseHeartbeat)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			if _, err := fmt.Fprint(w, ": keep-alive\n\n"); err != nil || rc.Flush() != nil {
				return
			}
		case ev, open := <-events:
			if !open {
				return
			}
			if !send(ev) || ev.Final {
				return
			}
		}
	}
}

func pagination(limitStr, offsetStr string) (int, int, error) {
	limit, offset := defaultListLimit, 0
	if limitStr != "" {
		v, err := strconv.Atoi(limitStr)
		if err != nil || v < 1 || v > 1000 {
			return 0, 0, fmt.Errorf("limit must be an integer between 1 and 1000")
		}
		limit = v
	}
	if offsetStr != "" {
		v, err := strconv.Atoi(offsetStr)
		if err != nil || v < 0 {
			return 0, 0, fmt.Errorf("offset must be a non-negative integer")
		}
		offset = v
	}
	return limit, offset, nil
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("invalid JSON body: %w", err)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// RoutePrefix is the path prefix served by APIHandler.
const RoutePrefix = "/replaylab/"

// IsAPIPath reports whether a path belongs to the Replay Lab API.
func IsAPIPath(path string) bool {
	return path == strings.TrimSuffix(RoutePrefix, "/") || strings.HasPrefix(path, RoutePrefix)
}
