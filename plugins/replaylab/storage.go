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
}

// Storage abstracts the persistence backend for captures and experiments.
type Storage interface {
	SaveCapture(record *CaptureRecord) error
	SaveCapturesBatch(records []*CaptureRecord) error
	GetCaptures(filter CaptureFilter) ([]*CaptureRecord, error)
	GetCaptureByID(id string) (*CaptureRecord, error)
	GetCaptureCount() (int64, error)
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

	// Configure connection pool for SQLite concurrency
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

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
			hash_only BOOLEAN
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
	return nil
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
			is_streaming, tag, hash_only
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, r := range records {
		_, err := stmt.Exec(
			r.ID, r.RequestID, r.Timestamp.UTC(), r.VirtualKey, r.Tenant, r.Provider, r.Model,
			r.MessagesJSON, r.ParamsJSON, r.ResponseJSON, r.PromptTokens, r.CompletionTokens,
			r.TotalTokens, r.LatencyMs, r.CostEstimate, r.StatusCode, r.ErrorMessage,
			r.IsStreaming, r.Tag, r.HashOnly,
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
		conditions = append(conditions, "model = ?")
		args = append(args, filter.Model)
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

	query := `
		SELECT id, request_id, timestamp, virtual_key, tenant, provider, model,
		       messages_json, params_json, response_json, prompt_tokens, completion_tokens,
		       total_tokens, latency_ms, cost_estimate, status_code, error_message,
		       is_streaming, tag, hash_only
		FROM captures
	`
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY timestamp DESC"

	limit := filter.Limit
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	query += fmt.Sprintf(" LIMIT %d OFFSET %d", limit, filter.Offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []*CaptureRecord
	for rows.Next() {
		r := &CaptureRecord{}
		var ts time.Time
		err := rows.Scan(
			&r.ID, &r.RequestID, &ts, &r.VirtualKey, &r.Tenant, &r.Provider, &r.Model,
			&r.MessagesJSON, &r.ParamsJSON, &r.ResponseJSON, &r.PromptTokens, &r.CompletionTokens,
			&r.TotalTokens, &r.LatencyMs, &r.CostEstimate, &r.StatusCode, &r.ErrorMessage,
			&r.IsStreaming, &r.Tag, &r.HashOnly,
		)
		if err != nil {
			return nil, err
		}
		r.Timestamp = ts
		records = append(records, r)
	}
	return records, rows.Err()
}

// GetCaptureByID retrieves a specific record.
func (s *SQLiteStorage) GetCaptureByID(id string) (*CaptureRecord, error) {
	row := s.db.QueryRow(`
		SELECT id, request_id, timestamp, virtual_key, tenant, provider, model,
		       messages_json, params_json, response_json, prompt_tokens, completion_tokens,
		       total_tokens, latency_ms, cost_estimate, status_code, error_message,
		       is_streaming, tag, hash_only
		FROM captures WHERE id = ?
	`, id)

	r := &CaptureRecord{}
	var ts time.Time
	err := row.Scan(
		&r.ID, &r.RequestID, &ts, &r.VirtualKey, &r.Tenant, &r.Provider, &r.Model,
		&r.MessagesJSON, &r.ParamsJSON, &r.ResponseJSON, &r.PromptTokens, &r.CompletionTokens,
		&r.TotalTokens, &r.LatencyMs, &r.CostEstimate, &r.StatusCode, &r.ErrorMessage,
		&r.IsStreaming, &r.Tag, &r.HashOnly,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	r.Timestamp = ts
	return r, nil
}

// GetCaptureCount returns total number of captures stored.
func (s *SQLiteStorage) GetCaptureCount() (int64, error) {
	var count int64
	err := s.db.QueryRow("SELECT COUNT(*) FROM captures").Scan(&count)
	return count, err
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
	if limit <= 0 {
		limit = 20
	}
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
		if err := json.Unmarshal([]byte(dataStr), &exp); err == nil {
			experiments = append(experiments, &exp)
		}
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
