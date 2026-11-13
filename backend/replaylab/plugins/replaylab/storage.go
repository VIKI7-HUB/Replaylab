package replaylab

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// CaptureFilter defines search parameters for querying stored traffic.
type CaptureFilter struct {
	Model      string
	Tag        string
	VirtualKey string
	Limit      int
	Offset     int
	Since      *time.Time
	// Replayable restricts results to successful, non-hashed, complete
	// captures, i.e. the ones that can be meaningfully replayed and scored.
	Replayable bool
	// Streamed, when set, keeps only streamed (true) or only non-streamed
	// (false) captures. Nil matches both.
	Streamed *bool
}

// TrafficVolume summarises how much traffic was captured for a model.
type TrafficVolume struct {
	Count int64
	First time.Time
	Last  time.Time
}

// MonthlyEstimate extrapolates the captured rate to a 30-day volume. Spans
// shorter than a day are treated as a full day to avoid wild projections.
func (v TrafficVolume) MonthlyEstimate() int64 {
	if v.Count == 0 {
		return 0
	}
	span := v.Last.Sub(v.First)
	if span < 24*time.Hour {
		span = 24 * time.Hour
	}
	perDay := float64(v.Count) / span.Hours() * 24
	return int64(perDay * 30)
}

// Storage abstracts the persistence backend for captures and experiments.
type Storage interface {
	SaveCapture(record *CaptureRecord) error
	SaveCapturesBatch(records []*CaptureRecord) error
	GetCaptures(filter CaptureFilter) ([]*CaptureRecord, error)
	GetCaptureByID(id string) (*CaptureRecord, error)
	GetCaptureCount() (int64, error)
	GetTrafficVolume(model string) (TrafficVolume, error)
	SaveExperiment(exp *Experiment) error
	GetExperiment(id string) (*Experiment, error)
	ListExperiments(limit, offset int) ([]*Experiment, error)
	PurgeExpiredCaptures(retentionDays int) (int64, error)
	Close() error
}

// SQLiteStorage implements Storage using SQLite.
type SQLiteStorage struct {
	db *sql.DB
}

// NewSQLiteStorage initializes database schema and returns a ready Storage instance.
func NewSQLiteStorage(dbPath string) (*SQLiteStorage, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}

	// SQLite allows a single writer; one connection serialises access and
	// WAL + busy_timeout keep readers from failing under write load.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	for _, pragma := range []string{"PRAGMA journal_mode=WAL", "PRAGMA busy_timeout=5000"} {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to configure sqlite (%s): %w", pragma, err)
		}
	}

	s := &SQLiteStorage{db: db}
	if err := s.initSchema(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *SQLiteStorage) initSchema() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS captures (
			id TEXT PRIMARY KEY,
			request_id TEXT NOT NULL,
			timestamp DATETIME NOT NULL,
			virtual_key TEXT,
			tenant TEXT,
			provider TEXT NOT NULL,
			model TEXT NOT NULL,
			messages_json TEXT NOT NULL,
			params_json TEXT,
			response_json TEXT,
			prompt_tokens INTEGER,
			completion_tokens INTEGER,
			total_tokens INTEGER,
			latency_ms INTEGER,
			cost_estimate REAL,
			status_code INTEGER,
			error_message TEXT,
			is_streaming BOOLEAN,
			tag TEXT,
			hash_only BOOLEAN,
			completion_state TEXT,
			time_to_first_token_ms INTEGER,
			usage_estimated BOOLEAN
		);`,
		`CREATE INDEX IF NOT EXISTS idx_captures_timestamp ON captures(timestamp DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_captures_model ON captures(model);`,
		`CREATE INDEX IF NOT EXISTS idx_captures_tag ON captures(tag);`,
		`CREATE TABLE IF NOT EXISTS experiments (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			status TEXT NOT NULL,
			original_model TEXT NOT NULL,
			data_json TEXT NOT NULL,
			created_at DATETIME NOT NULL,
			completed_at DATETIME
		);`,
		`CREATE INDEX IF NOT EXISTS idx_experiments_created_at ON experiments(created_at DESC);`,
	}

	for _, q := range queries {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("schema initialization failed on query [%s]: %w", q, err)
		}
	}
	return s.addMissingCaptureColumns()
}

// addMissingCaptureColumns upgrades databases created before streamed
// capture. Old rows get NULLs, which read back as a complete, non-estimated
// capture.
func (s *SQLiteStorage) addMissingCaptureColumns() error {
	rows, err := s.db.Query("PRAGMA table_info(captures)")
	if err != nil {
		return fmt.Errorf("reading captures schema: %w", err)
	}
	have := map[string]bool{}
	for rows.Next() {
		var (
			cid, notNull, pk int
			name, typ        string
			dflt             sql.NullString
		)
		if err := rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk); err != nil {
			rows.Close()
			return fmt.Errorf("reading captures schema: %w", err)
		}
		have[name] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("reading captures schema: %w", err)
	}
	for _, col := range []struct{ name, typ string }{
		{"completion_state", "TEXT"},
		{"time_to_first_token_ms", "INTEGER"},
		{"usage_estimated", "BOOLEAN"},
	} {
		if have[col.name] {
			continue
		}
		if _, err := s.db.Exec("ALTER TABLE captures ADD COLUMN " + col.name + " " + col.typ); err != nil {
			return fmt.Errorf("adding captures.%s: %w", col.name, err)
		}
	}
	return nil
}

// captureColumns is the SELECT list scanCapture reads, in order.
const captureColumns = `id, request_id, timestamp, virtual_key, tenant, provider, model,
	       messages_json, params_json, response_json, prompt_tokens, completion_tokens,
	       total_tokens, latency_ms, cost_estimate, status_code, error_message,
	       is_streaming, tag, hash_only,
	       COALESCE(completion_state, 'complete'), time_to_first_token_ms, COALESCE(usage_estimated, 0)`

type rowScanner interface{ Scan(dest ...any) error }

func scanCapture(row rowScanner) (*CaptureRecord, error) {
	r := &CaptureRecord{}
	var (
		ts   time.Time
		ttft sql.NullInt64
	)
	err := row.Scan(
		&r.ID, &r.RequestID, &ts, &r.VirtualKey, &r.Tenant, &r.Provider, &r.Model,
		&r.MessagesJSON, &r.ParamsJSON, &r.ResponseJSON, &r.PromptTokens, &r.CompletionTokens,
		&r.TotalTokens, &r.LatencyMs, &r.CostEstimate, &r.StatusCode, &r.ErrorMessage,
		&r.IsStreaming, &r.Tag, &r.HashOnly,
		&r.CompletionState, &ttft, &r.UsageEstimated,
	)
	if err != nil {
		return nil, err
	}
	r.Timestamp = ts
	if ttft.Valid {
		v := ttft.Int64
		r.TimeToFirstTokenMs = &v
	}
	return r, nil
}

// SaveCapture inserts a single record.
func (s *SQLiteStorage) SaveCapture(r *CaptureRecord) error {
	return s.SaveCapturesBatch([]*CaptureRecord{r})
}

// SaveCapturesBatch writes records atomically in a transaction.
func (s *SQLiteStorage) SaveCapturesBatch(records []*CaptureRecord) error {
	if len(records) == 0 {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO captures (
			id, request_id, timestamp, virtual_key, tenant, provider, model,
			messages_json, params_json, response_json, prompt_tokens, completion_tokens,
			total_tokens, latency_ms, cost_estimate, status_code, error_message,
			is_streaming, tag, hash_only, completion_state, time_to_first_token_ms, usage_estimated
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, r := range records {
		state := r.CompletionState
		if state == "" {
			state = CompletionStateComplete
		}
		var ttft any
		if r.TimeToFirstTokenMs != nil {
			ttft = *r.TimeToFirstTokenMs
		}
		_, err := stmt.Exec(
			r.ID, r.RequestID, r.Timestamp.UTC(), r.VirtualKey, r.Tenant, r.Provider, r.Model,
			r.MessagesJSON, r.ParamsJSON, r.ResponseJSON, r.PromptTokens, r.CompletionTokens,
			r.TotalTokens, r.LatencyMs, r.CostEstimate, r.StatusCode, r.ErrorMessage,
			r.IsStreaming, r.Tag, r.HashOnly, state, ttft, r.UsageEstimated,
		)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetCaptures queries captures based on filters.
func (s *SQLiteStorage) GetCaptures(filter CaptureFilter) ([]*CaptureRecord, error) {
	var conditions []string
	var args []any

	if filter.Model != "" {
		// Bifrost records the provider and model separately, so a model named
		// the way Bifrost routes it ("groq/openai/gpt-oss-20b") matches too.
		conditions = append(conditions, "(model = ? OR provider || '/' || model = ?)")
		args = append(args, filter.Model, filter.Model)
	}
	if filter.Tag != "" {
		conditions = append(conditions, "tag = ?")
		args = append(args, filter.Tag)
	}
	if filter.VirtualKey != "" {
		conditions = append(conditions, "virtual_key = ?")
		args = append(args, filter.VirtualKey)
	}
	if filter.Since != nil {
		conditions = append(conditions, "timestamp >= ?")
		args = append(args, filter.Since.UTC())
	}
	if filter.Streamed != nil {
		conditions = append(conditions, "COALESCE(is_streaming, 0) = ?")
		args = append(args, *filter.Streamed)
	}
	if filter.Replayable {
		conditions = append(conditions, "status_code BETWEEN 200 AND 299", "hash_only = 0",
			"COALESCE(completion_state, 'complete') = 'complete'")
	}

	query := "SELECT " + captureColumns + " FROM captures"
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY timestamp DESC"

	limit := filter.Limit
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	offset := max(filter.Offset, 0)
	query += " LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []*CaptureRecord
	for rows.Next() {
		r, err := scanCapture(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

// GetCaptureByID retrieves a specific record.
func (s *SQLiteStorage) GetCaptureByID(id string) (*CaptureRecord, error) {
	r, err := scanCapture(s.db.QueryRow("SELECT "+captureColumns+" FROM captures WHERE id = ?", id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return r, nil
}

// GetCaptureCount returns total number of captures stored.
func (s *SQLiteStorage) GetCaptureCount() (int64, error) {
	var count int64
	err := s.db.QueryRow("SELECT COUNT(*) FROM captures").Scan(&count)
	return count, err
}

// GetTrafficVolume reports how many successful captures exist for a model
// and the time span they cover. An empty model matches all captures.
func (s *SQLiteStorage) GetTrafficVolume(model string) (TrafficVolume, error) {
	where := " WHERE status_code BETWEEN 200 AND 299"
	var args []any
	if model != "" {
		where += " AND (model = ? OR provider || '/' || model = ?)"
		args = append(args, model, model)
	}
	var v TrafficVolume
	if err := s.db.QueryRow("SELECT COUNT(*) FROM captures"+where, args...).Scan(&v.Count); err != nil {
		return v, err
	}
	if v.Count == 0 {
		return v, nil
	}
	// Read the boundary rows rather than MIN/MAX aggregates: aggregates lose
	// the DATETIME column type, so the driver would return untyped text.
	for _, q := range []struct {
		order string
		dst   *time.Time
	}{{"ASC", &v.First}, {"DESC", &v.Last}} {
		query := "SELECT timestamp FROM captures" + where + " ORDER BY timestamp " + q.order + " LIMIT 1"
		if err := s.db.QueryRow(query, args...).Scan(q.dst); err != nil {
			return v, err
		}
	}
	return v, nil
}

// SaveExperiment stores or updates an experiment record.
func (s *SQLiteStorage) SaveExperiment(exp *Experiment) error {
	dataBytes, err := json.Marshal(exp)
	if err != nil {
		return err
	}

	var completedAt any
	if exp.CompletedAt != nil {
		completedAt = exp.CompletedAt.UTC()
	}

	_, err = s.db.Exec(`
		INSERT OR REPLACE INTO experiments (id, name, status, original_model, data_json, created_at, completed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, exp.ID, exp.Name, exp.Status, exp.OriginalModel, string(dataBytes), exp.CreatedAt.UTC(), completedAt)
	return err
}

// InterruptedStopReason is the stop reason given to experiments that were
// still pending or running when the previous process stopped.
const InterruptedStopReason = "interrupted: Replay Lab stopped before the run finished; partial results kept"

// FailInterruptedExperiments marks every pending or running experiment as
// failed with InterruptedStopReason, keeping its partial progress, and
// returns how many it marked. Runs live in memory, so at startup any such
// experiment belongs to a process that is gone and would otherwise stay
// "running" forever. Call it only before this process starts experiments.
func (s *SQLiteStorage) FailInterruptedExperiments(now time.Time) (int, error) {
	rows, err := s.db.Query("SELECT data_json FROM experiments WHERE status IN (?, ?)", StatusPending, StatusRunning)
	if err != nil {
		return 0, err
	}
	var stale []*Experiment
	for rows.Next() {
		var dataStr string
		if err := rows.Scan(&dataStr); err != nil {
			rows.Close()
			return 0, err
		}
		var exp Experiment
		if err := json.Unmarshal([]byte(dataStr), &exp); err != nil {
			rows.Close()
			return 0, fmt.Errorf("corrupt experiment record: %w", err)
		}
		stale = append(stale, &exp)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	now = now.UTC()
	for _, exp := range stale {
		exp.Status = StatusFailed
		exp.StopReason = InterruptedStopReason
		exp.CompletedAt = &now
		if err := s.SaveExperiment(exp); err != nil {
			return 0, err
		}
	}
	return len(stale), nil
}

// GetExperiment retrieves an experiment by ID.
func (s *SQLiteStorage) GetExperiment(id string) (*Experiment, error) {
	row := s.db.QueryRow("SELECT data_json FROM experiments WHERE id = ?", id)
	var dataStr string
	err := row.Scan(&dataStr)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var exp Experiment
	if err := json.Unmarshal([]byte(dataStr), &exp); err != nil {
		return nil, err
	}
	return &exp, nil
}

// ListExperiments retrieves paginated experiment records.
func (s *SQLiteStorage) ListExperiments(limit, offset int) ([]*Experiment, error) {
	if limit <= 0 || limit > 200 {
		limit = 20
	}
	offset = max(offset, 0)
	rows, err := s.db.Query(
		"SELECT data_json FROM experiments ORDER BY created_at DESC LIMIT ? OFFSET ?",
		limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var experiments []*Experiment
	for rows.Next() {
		var dataStr string
		if err := rows.Scan(&dataStr); err != nil {
			return nil, err
		}
		var exp Experiment
		if err := json.Unmarshal([]byte(dataStr), &exp); err != nil {
			return nil, fmt.Errorf("corrupt experiment record: %w", err)
		}
		experiments = append(experiments, &exp)
	}
	return experiments, rows.Err()
}

// PurgeExpiredCaptures deletes captures older than retentionDays.
func (s *SQLiteStorage) PurgeExpiredCaptures(retentionDays int) (int64, error) {
	if retentionDays <= 0 {
		return 0, nil
	}
	cutoff := time.Now().AddDate(0, 0, -retentionDays).UTC()
	res, err := s.db.Exec("DELETE FROM captures WHERE timestamp < ?", cutoff)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// Close gracefully closes the SQLite database connection.
func (s *SQLiteStorage) Close() error {
	return s.db.Close()
}
