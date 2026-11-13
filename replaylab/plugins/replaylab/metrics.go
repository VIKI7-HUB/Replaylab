package replaylab

import (
	"fmt"
	"strings"
)

// FormatPrometheus renders Replay Lab counters in the Prometheus text format.
func FormatPrometheus(storage Storage, q *CaptureQueue, engine *ReplayEngine) string {
	var sb strings.Builder
	metric := func(name, kind, help string, value any) {
		fmt.Fprintf(&sb, "# HELP %s %s\n# TYPE %s %s\n%s %v\n\n", name, help, name, kind, name, value)
	}

	captured, dropped := q.GetMetrics()
	stored, _ := storage.GetCaptureCount()
	replays, failed := engine.Metrics()
	streamed, truncated := q.StreamMetrics()

	metric("bifrost_replaylab_captures_total", "counter", "Captures accepted into the write queue", captured)
	metric("bifrost_replaylab_captures_dropped_total", "counter", "Captures dropped because the write queue was full", dropped)
	metric("bifrost_replaylab_capture_write_errors_total", "counter", "Captures that failed to persist", q.WriteErrors())
	metric("bifrost_replaylab_stream_captures_total", "counter", "Streamed captures accepted into the write queue", streamed)
	metric("bifrost_replaylab_stream_captures_truncated_total", "counter", "Streamed captures cut off by the per-stream content cap", truncated)
	metric("bifrost_replaylab_captures_stored", "gauge", "Captures currently stored", stored)
	metric("bifrost_replaylab_replays_total", "counter", "Replays sent to candidate models", replays)
	metric("bifrost_replaylab_replays_failed_total", "counter", "Replays that returned an error", failed)
	metric("bifrost_replaylab_experiments_active", "gauge", "Experiments currently running", engine.ActiveExperiments())
	return sb.String()
}
