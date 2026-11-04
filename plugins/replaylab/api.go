package replaylab

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

// APIHandler dispatches REST endpoints for Replay Lab.
type APIHandler struct {
	storage Storage
	engine  *ReplayEngine
	queue   *CaptureQueue
	metrics *MetricsTracker
	config  Config
	cfgMu   sync.RWMutex
	mux     *http.ServeMux
}

// NewAPIHandler sets up routing and handlers.
func NewAPIHandler(storage Storage, engine *ReplayEngine, queue *CaptureQueue, cfg Config) *APIHandler {
	h := &APIHandler{
		storage: storage,
		engine:  engine,
		queue:   queue,
		metrics: NewMetricsTracker(),
		config:  cfg,
		mux:     http.NewServeMux(),
	}
	h.registerRoutes()
	return h
}

func (h *APIHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Enable permissive CORS
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	h.mux.ServeHTTP(w, r)
}

func (h *APIHandler) registerRoutes() {
	h.mux.HandleFunc("/replaylab/health", h.handleHealth)
	h.mux.HandleFunc("/replaylab/metrics", h.handleMetrics)
	h.mux.HandleFunc("/replaylab/config", h.handleConfig)
	h.mux.HandleFunc("/replaylab/captures", h.handleCaptures)
	h.mux.HandleFunc("/replaylab/captures/", h.handleCaptureByID)
	h.mux.HandleFunc("/replaylab/experiments", h.handleExperiments)
	h.mux.HandleFunc("/replaylab/experiments/", h.handleExperimentSubroutes)
}

func (h *APIHandler) handleHealth(w http.ResponseWriter, r *http.Request) {
	count, _ := h.storage.GetCaptureCount()
	resp := map[string]any{
		"status":          "healthy",
		"service":         "bifrost-replaylab",
		"version":         "1.0.0",
		"stored_captures": count,
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *APIHandler) handleMetrics(w http.ResponseWriter, r *http.Request) {
	metricsText := h.metrics.FormatPrometheus(h.storage, h.queue)
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(metricsText))
}

func (h *APIHandler) handleConfig(w http.ResponseWriter, r *http.Request) {
	h.cfgMu.Lock()
	defer h.cfgMu.Unlock()

	if r.Method == http.MethodPut {
		var updated Config
		if err := json.NewDecoder(r.Body).Decode(&updated); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		h.config = updated
		writeJSON(w, http.StatusOK, h.config)
		return
	}

	writeJSON(w, http.StatusOK, h.config)
}

func (h *APIHandler) handleCaptures(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))

	captures, err := h.storage.GetCaptures(CaptureFilter{
		Model:      q.Get("model"),
		Tag:        q.Get("tag"),
		VirtualKey: q.Get("virtual_key"),
		Limit:      limit,
		Offset:     offset,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if captures == nil {
		captures = []*CaptureRecord{}
	}
	writeJSON(w, http.StatusOK, captures)
}

func (h *APIHandler) handleCaptureByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/replaylab/captures/")
	if id == "" {
		http.Error(w, "missing capture id", http.StatusBadRequest)
		return
	}

	cap, err := h.storage.GetCaptureByID(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if cap == nil {
		http.Error(w, "capture not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, cap)
}

func (h *APIHandler) handleExperiments(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var req CreateExperimentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid payload: "+err.Error(), http.StatusBadRequest)
			return
		}

		exp, err := h.engine.StartExperiment(req)
		if err != nil {
			http.Error(w, "failed to start experiment: "+err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusAccepted, exp)
		return
	}

	if r.Method == http.MethodGet {
		q := r.URL.Query()
		limit, _ := strconv.Atoi(q.Get("limit"))
		offset, _ := strconv.Atoi(q.Get("offset"))

		experiments, err := h.storage.ListExperiments(limit, offset)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if experiments == nil {
			experiments = []*Experiment{}
		}
		writeJSON(w, http.StatusOK, experiments)
		return
	}

	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

func (h *APIHandler) handleExperimentSubroutes(w http.ResponseWriter, r *http.Request) {
	subpath := strings.TrimPrefix(r.URL.Path, "/replaylab/experiments/")
	parts := strings.Split(subpath, "/")
	expID := parts[0]

	if len(parts) == 1 {
		// GET /replaylab/experiments/{id}
		exp, err := h.storage.GetExperiment(expID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if exp == nil {
			http.Error(w, "experiment not found", http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, exp)
		return
	}

	action := parts[1]
	switch action {
	case "cancel":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		cancelled := h.engine.CancelExperiment(expID)
		writeJSON(w, http.StatusOK, map[string]any{"cancelled": cancelled, "experiment_id": expID})

	case "report":
		exp, err := h.storage.GetExperiment(expID)
		if err != nil || exp == nil {
			http.Error(w, "experiment not found", http.StatusNotFound)
			return
		}
		if r.URL.Query().Get("format") == "markdown" {
			w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(exp.SummaryMarkdown))
			return
		}
		writeJSON(w, http.StatusOK, exp)

	case "events":
		// SSE Progress Stream
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		ch := h.engine.SubscribeEvents(expID)
		for {
			select {
			case <-r.Context().Done():
				return
			case event, ok := <-ch:
				if !ok {
					return
				}
				data, _ := json.Marshal(event)
				fmt.Fprintf(w, "data: %s\n\n", data)
				flusher.Flush()
			}
		}

	default:
		http.NotFound(w, r)
	}
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
