package tests

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/maximhq/bifrost/plugins/replaylab/plugins/replaylab"
)

// An experiment that was running when the process stopped can never finish:
// runs live in memory. On the next start it must not stay "running" forever.
func TestExperimentInterruptedByRestartIsMarkedFailed(t *testing.T) {
	cfg := replaylab.DefaultConfig()
	cfg.DBPath = filepath.Join(t.TempDir(), "restart.db")

	// The state a process leaves behind when it stops mid-run.
	storage, err := replaylab.NewSQLiteStorage(cfg.DBPath)
	if err != nil {
		t.Fatalf("NewSQLiteStorage: %v", err)
	}
	result := replaylab.ExperimentResult{CandidateModel: "gpt-4o-mini", TotalRequests: 3, SuccessfulRequests: 3}
	for _, exp := range []*replaylab.Experiment{
		{ID: "exp-running", Name: "mid-run", Status: replaylab.StatusRunning, OriginalModel: "gpt-4o",
			CandidateModels: []string{"gpt-4o-mini"}, CreatedAt: time.Now().Add(-time.Minute),
			TotalReplays: 10, CompletedReplays: 3, SpentDollars: 0.01, Results: []replaylab.ExperimentResult{result}},
		{ID: "exp-pending", Name: "queued", Status: replaylab.StatusPending, OriginalModel: "gpt-4o",
			CandidateModels: []string{"gpt-4o-mini"}, CreatedAt: time.Now().Add(-time.Minute)},
		{ID: "exp-done", Name: "finished", Status: replaylab.StatusCompleted, OriginalModel: "gpt-4o",
			CandidateModels: []string{"gpt-4o-mini"}, CreatedAt: time.Now().Add(-time.Hour)},
	} {
		if err := storage.SaveExperiment(exp); err != nil {
			t.Fatalf("SaveExperiment: %v", err)
		}
	}
	if err := storage.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	p, err := replaylab.NewPlugin(cfg)
	if err != nil {
		t.Fatalf("NewPlugin: %v", err)
	}
	defer p.Cleanup()

	for _, id := range []string{"exp-running", "exp-pending"} {
		exp, err := p.Storage().GetExperiment(id)
		if err != nil || exp == nil {
			t.Fatalf("GetExperiment(%s): %v, %v", id, exp, err)
		}
		if exp.Status != replaylab.StatusFailed {
			t.Errorf("%s: status = %q after restart, want %q", id, exp.Status, replaylab.StatusFailed)
		}
		if !strings.Contains(exp.StopReason, "interrupted") {
			t.Errorf("%s: stop_reason = %q, want it to say the run was interrupted", id, exp.StopReason)
		}
		if exp.CompletedAt == nil {
			t.Errorf("%s: completed_at not set", id)
		}
	}

	running, _ := p.Storage().GetExperiment("exp-running")
	if running.CompletedReplays != 3 || len(running.Results) != 1 || running.SpentDollars != 0.01 {
		t.Errorf("partial progress lost: completed=%d results=%d spent=%v",
			running.CompletedReplays, len(running.Results), running.SpentDollars)
	}
	done, _ := p.Storage().GetExperiment("exp-done")
	if done.Status != replaylab.StatusCompleted || done.StopReason != "" {
		t.Errorf("finished experiment changed: status=%q stop_reason=%q", done.Status, done.StopReason)
	}
}
