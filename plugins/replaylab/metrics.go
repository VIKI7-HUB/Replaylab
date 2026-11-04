package replaylab

import (
	"fmt"
	"strings"
	"sync/atomic"
)

// MetricsTracker aggregates runtime counters for Prometheus export.
type MetricsTracker struct {
	capturesTotal        atomic.Int64
	capturesDroppedTotal atomic.Int64
	replaysTotal         atomic.Int64
	replaysFailedTotal   atomic.Int64
}

// NewMetricsTracker creates a tracker.
func NewMetricsTracker() *MetricsTracker {
	return &MetricsTracker{}
}

// FormatPrometheus formats Prometheus-compatible metrics text.
func (m *MetricsTracker) FormatPrometheus(storage Storage, q *CaptureQueue) string {
	var sb strings.Builder

	capTotal, droppedTotal := q.GetMetrics()
	count, _ := storage.GetCaptureCount()

	sb.WriteString("# HELP bifrost_replaylab_captures_total Total number of traffic captures enqueued\n")
	sb.WriteString("# TYPE bifrost_replaylab_captures_total counter\n")
	sb.WriteString(fmt.Sprintf("bifrost_replaylab_captures_total %d\n\n", capTotal))

	sb.WriteString("# HELP bifrost_replaylab_captures_dropped_total Total number of captures dropped due to ring-buffer saturation\n")
	sb.WriteString("# TYPE bifrost_replaylab_captures_dropped_total counter\n")
	sb.WriteString(fmt.Sprintf("bifrost_replaylab_captures_dropped_total %d\n\n", droppedTotal))

	sb.WriteString("# HELP bifrost_replaylab_captures_stored_count Current count of stored captures in database\n")
	sb.WriteString("# TYPE bifrost_replaylab_captures_stored_count gauge\n")
	sb.WriteString(fmt.Sprintf("bifrost_replaylab_captures_stored_count %d\n\n", count))

	return sb.String()
}
