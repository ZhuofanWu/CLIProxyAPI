package usage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"

	coreusage "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/usage"
	log "github.com/sirupsen/logrus"
)

// RequestStatistics persists usage records and serves aggregated statistics from SQLite.
type RequestStatistics struct {
	mu     sync.RWMutex
	dbPath string
}

const usageWriteTimeout = 5 * time.Second

// NewRequestStatistics constructs an empty statistics store.
func NewRequestStatistics() *RequestStatistics { return &RequestStatistics{} }

// Configure prepares the SQLite database under authDir.
func (s *RequestStatistics) Configure(authDir string) error {
	if s == nil {
		return nil
	}
	trimmedAuthDir := strings.TrimSpace(authDir)
	if trimmedAuthDir == "" {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.dbPath = ""
		return nil
	}
	if err := os.MkdirAll(trimmedAuthDir, 0o700); err != nil {
		return fmt.Errorf("usage statistics: create auth directory: %w", err)
	}
	path := filepath.Join(trimmedAuthDir, usageDatabaseFileName)
	path, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("usage statistics: resolve database path: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dbPath == path {
		return nil
	}

	db, err := openUsageDatabase(path)
	if err != nil {
		return fmt.Errorf("usage statistics: open database: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return fmt.Errorf("usage statistics: ping database: %w", err)
	}
	if err := configureUsageSchema(ctx, db); err != nil {
		_ = db.Close()
		return err
	}
	s.dbPath = path
	_ = db.Close()
	return nil
}

// DatabasePath returns the configured SQLite file path.
func (s *RequestStatistics) DatabasePath() string {
	if s == nil {
		return ""
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.dbPath
}

// Close clears the configured database path.
func (s *RequestStatistics) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dbPath = ""
	return nil
}

// Record ingests a new usage record and updates the SQLite aggregates.
func (s *RequestStatistics) Record(ctx context.Context, record coreusage.Record) {
	if s == nil {
		return
	}
	if !statisticsEnabled.Load() {
		return
	}
	persistedRecord := recordFromUsageRecord(ctx, record)
	writeCtx, cancel := context.WithTimeout(context.Background(), usageWriteTimeout)
	defer cancel()
	if err := s.insertRecord(writeCtx, persistedRecord); err != nil {
		log.Errorf("usage statistics: persist record: %v", err)
	}
}

// Snapshot returns a snapshot of the aggregated statistics.
func (s *RequestStatistics) Snapshot() StatisticsSnapshot {
	snapshot, err := s.SnapshotContext(context.Background())
	if err != nil {
		log.Errorf("usage statistics: snapshot: %v", err)
	}
	return snapshot
}

// SnapshotContext returns a snapshot of the aggregated statistics.
func (s *RequestStatistics) SnapshotContext(ctx context.Context) (StatisticsSnapshot, error) {
	return s.SnapshotContextWithOptions(ctx, SnapshotOptions{})
}

// SnapshotContextWithOptions returns a snapshot of the aggregated statistics.
func (s *RequestStatistics) SnapshotContextWithOptions(ctx context.Context, options SnapshotOptions) (StatisticsSnapshot, error) {
	db, err := s.openDatabase()
	if err != nil {
		if errors.Is(err, sql.ErrConnDone) {
			return StatisticsSnapshot{}, nil
		}
		return StatisticsSnapshot{}, err
	}
	defer db.Close()

	result := newStatisticsSnapshot()
	options = normalizeSnapshotOptions(options)

	if options.Since.IsZero() {
		if err := querySummary(ctx, db, &result); err != nil {
			return StatisticsSnapshot{}, err
		}
		if err := queryAPIRollups(ctx, db, &result); err != nil {
			return StatisticsSnapshot{}, err
		}
		if err := queryModelRollups(ctx, db, &result); err != nil {
			return StatisticsSnapshot{}, err
		}
		if err := queryDailyRollups(ctx, db, &result); err != nil {
			return StatisticsSnapshot{}, err
		}
		if err := queryHourlyRollups(ctx, db, &result); err != nil {
			return StatisticsSnapshot{}, err
		}
	} else {
		filter := buildUsageRecordFilter(options.Since)
		if err := querySummaryFromRecords(ctx, db, &result, filter); err != nil {
			return StatisticsSnapshot{}, err
		}
		if err := queryAPIRollupsFromRecords(ctx, db, &result, filter); err != nil {
			return StatisticsSnapshot{}, err
		}
		if err := queryModelRollupsFromRecords(ctx, db, &result, filter); err != nil {
			return StatisticsSnapshot{}, err
		}
		if err := queryDailyRollupsFromRecords(ctx, db, &result, filter); err != nil {
			return StatisticsSnapshot{}, err
		}
		if err := queryHourlyRollupsFromRecords(ctx, db, &result, filter); err != nil {
			return StatisticsSnapshot{}, err
		}
	}
	if err := queryModelDetails(ctx, db, &result, options); err != nil {
		return StatisticsSnapshot{}, err
	}

	return result, nil
}

// ExportRecords returns all persisted usage records in a stable order.
func (s *RequestStatistics) ExportRecords(ctx context.Context) ([]PersistedRecord, error) {
	db, err := s.openDatabase()
	if err != nil {
		if errors.Is(err, sql.ErrConnDone) {
			return nil, nil
		}
		return nil, err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, `
		SELECT api_name, model_name, timestamp_utc, source, auth_index, failed,
		       input_tokens, output_tokens, reasoning_tokens, cached_tokens, total_tokens
		FROM usage_records
		ORDER BY timestamp_utc ASC, api_name ASC, model_name ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("usage statistics: export records: %w", err)
	}
	defer rows.Close()

	records := make([]PersistedRecord, 0, 128)
	for rows.Next() {
		var (
			record       PersistedRecord
			timestampUTC string
			failedInt    int64
		)
		if err := rows.Scan(
			&record.APIName,
			&record.ModelName,
			&timestampUTC,
			&record.Source,
			&record.AuthIndex,
			&failedInt,
			&record.Tokens.InputTokens,
			&record.Tokens.OutputTokens,
			&record.Tokens.ReasoningTokens,
			&record.Tokens.CachedTokens,
			&record.Tokens.TotalTokens,
		); err != nil {
			return nil, fmt.Errorf("usage statistics: scan export record: %w", err)
		}
		parsedTime, err := time.Parse(time.RFC3339Nano, timestampUTC)
		if err != nil {
			return nil, fmt.Errorf("usage statistics: parse export timestamp: %w", err)
		}
		record.Timestamp = parsedTime.UTC()
		record.Failed = failedInt != 0
		record.Tokens = normaliseTokenStats(record.Tokens)
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("usage statistics: iterate export records: %w", err)
	}
	return records, nil
}

// MergeRecords imports persisted usage records.
func (s *RequestStatistics) MergeRecords(ctx context.Context, records []PersistedRecord) (MergeResult, error) {
	db, err := s.openDatabase()
	if err != nil {
		return MergeResult{}, err
	}
	defer db.Close()
	if len(records) == 0 {
		return MergeResult{}, nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return MergeResult{}, fmt.Errorf("usage statistics: begin import transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	result := MergeResult{}
	for _, record := range records {
		added, err := insertPersistedRecord(ctx, tx, normalizePersistedRecord(record))
		if err != nil {
			return MergeResult{}, err
		}
		if added {
			result.Added++
		} else {
			result.Skipped++
		}
	}
	if err := tx.Commit(); err != nil {
		return MergeResult{}, fmt.Errorf("usage statistics: commit import transaction: %w", err)
	}
	return result, nil
}

// MergeSnapshot merges a legacy exported statistics snapshot into the current store.
func (s *RequestStatistics) MergeSnapshot(snapshot StatisticsSnapshot) MergeResult {
	result, err := s.MergeSnapshotContext(context.Background(), snapshot)
	if err != nil {
		log.Errorf("usage statistics: merge snapshot: %v", err)
	}
	return result
}

// MergeSnapshotContext merges a legacy exported statistics snapshot into the current store.
func (s *RequestStatistics) MergeSnapshotContext(ctx context.Context, snapshot StatisticsSnapshot) (MergeResult, error) {
	records := make([]PersistedRecord, 0, 128)
	for apiName, apiSnapshot := range snapshot.APIs {
		apiName = strings.TrimSpace(apiName)
		if apiName == "" {
			continue
		}
		for modelName, modelSnapshot := range apiSnapshot.Models {
			modelName = strings.TrimSpace(modelName)
			if modelName == "" {
				modelName = "unknown"
			}
			for _, detail := range modelSnapshot.Details {
				records = append(records, normalizePersistedRecord(PersistedRecord{
					APIName:   apiName,
					ModelName: modelName,
					Timestamp: detail.Timestamp,
					Source:    detail.Source,
					AuthIndex: detail.AuthIndex,
					Failed:    detail.Failed,
					Tokens:    detail.Tokens,
				}))
			}
		}
	}
	return s.MergeRecords(ctx, records)
}

func (s *RequestStatistics) insertRecord(ctx context.Context, record PersistedRecord) error {
	db, err := s.openDatabase()
	if err != nil {
		return err
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("usage statistics: begin transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()
	if _, err := insertPersistedRecord(ctx, tx, record); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("usage statistics: commit transaction: %w", err)
	}
	return nil
}

func (s *RequestStatistics) openDatabase() (*sql.DB, error) {
	if s == nil {
		return nil, sql.ErrConnDone
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.dbPath == "" {
		return nil, sql.ErrConnDone
	}
	return openUsageDatabase(s.dbPath)
}

func openUsageDatabase(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := configureUsageConnection(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func configureUsageConnection(ctx context.Context, db *sql.DB) error {
	statements := []string{
		`PRAGMA journal_mode = WAL;`,
		`PRAGMA synchronous = FULL;`,
		`PRAGMA busy_timeout = 5000;`,
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("usage statistics: configure connection: %w", err)
		}
	}
	return nil
}

func configureUsageSchema(ctx context.Context, db *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS usage_summary (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			total_requests INTEGER NOT NULL DEFAULT 0,
			success_count INTEGER NOT NULL DEFAULT 0,
			failure_count INTEGER NOT NULL DEFAULT 0,
			total_tokens INTEGER NOT NULL DEFAULT 0
		);`,
		`INSERT OR IGNORE INTO usage_summary (id, total_requests, success_count, failure_count, total_tokens)
		 VALUES (1, 0, 0, 0, 0);`,
		`CREATE TABLE IF NOT EXISTS usage_api_rollups (
			api_name TEXT PRIMARY KEY,
			total_requests INTEGER NOT NULL DEFAULT 0,
			total_tokens INTEGER NOT NULL DEFAULT 0
		);`,
		`CREATE TABLE IF NOT EXISTS usage_model_rollups (
			api_name TEXT NOT NULL,
			model_name TEXT NOT NULL,
			total_requests INTEGER NOT NULL DEFAULT 0,
			total_tokens INTEGER NOT NULL DEFAULT 0,
			input_tokens INTEGER NOT NULL DEFAULT 0,
			output_tokens INTEGER NOT NULL DEFAULT 0,
			reasoning_tokens INTEGER NOT NULL DEFAULT 0,
			cached_tokens INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (api_name, model_name)
		);`,
		`CREATE TABLE IF NOT EXISTS usage_daily_rollups (
			day_key TEXT PRIMARY KEY,
			request_count INTEGER NOT NULL DEFAULT 0,
			total_tokens INTEGER NOT NULL DEFAULT 0
		);`,
		`CREATE TABLE IF NOT EXISTS usage_hourly_rollups (
			hour_key TEXT PRIMARY KEY,
			request_count INTEGER NOT NULL DEFAULT 0,
			total_tokens INTEGER NOT NULL DEFAULT 0
		);`,
		`CREATE TABLE IF NOT EXISTS usage_records (
			dedup_key TEXT PRIMARY KEY,
			api_name TEXT NOT NULL,
			model_name TEXT NOT NULL,
			timestamp_utc TEXT NOT NULL,
			source TEXT NOT NULL DEFAULT '',
			auth_index TEXT NOT NULL DEFAULT '',
			failed INTEGER NOT NULL DEFAULT 0,
			input_tokens INTEGER NOT NULL DEFAULT 0,
			output_tokens INTEGER NOT NULL DEFAULT 0,
			reasoning_tokens INTEGER NOT NULL DEFAULT 0,
			cached_tokens INTEGER NOT NULL DEFAULT 0,
			total_tokens INTEGER NOT NULL DEFAULT 0
		);`,
		`CREATE INDEX IF NOT EXISTS idx_usage_records_api_model_ts ON usage_records (api_name, model_name, timestamp_utc DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_usage_records_ts ON usage_records (timestamp_utc DESC);`,
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("usage statistics: initialize database: %w", err)
		}
	}
	return nil
}

func insertPersistedRecord(ctx context.Context, tx *sql.Tx, record PersistedRecord) (bool, error) {
	record = normalizePersistedRecord(record)
	detail := RequestDetail{
		Timestamp: record.Timestamp,
		Source:    record.Source,
		AuthIndex: record.AuthIndex,
		Tokens:    record.Tokens,
		Failed:    record.Failed,
	}
	dedup := dedupKey(record.APIName, record.ModelName, detail)
	failedInt := int64(0)
	failureDelta := int64(0)
	successDelta := int64(1)
	if record.Failed {
		failedInt = 1
		failureDelta = 1
		successDelta = 0
	}
	result, err := tx.ExecContext(ctx, `
		INSERT OR IGNORE INTO usage_records (
			dedup_key, api_name, model_name, timestamp_utc, source, auth_index, failed,
			input_tokens, output_tokens, reasoning_tokens, cached_tokens, total_tokens
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, dedup, record.APIName, record.ModelName, record.Timestamp.UTC().Format(time.RFC3339Nano), record.Source, record.AuthIndex, failedInt,
		record.Tokens.InputTokens, record.Tokens.OutputTokens, record.Tokens.ReasoningTokens, record.Tokens.CachedTokens, record.Tokens.TotalTokens)
	if err != nil {
		return false, fmt.Errorf("usage statistics: insert record: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("usage statistics: inspect inserted record: %w", err)
	}
	if rowsAffected == 0 {
		return false, nil
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE usage_summary
		SET total_requests = total_requests + 1,
		    success_count = success_count + ?,
		    failure_count = failure_count + ?,
		    total_tokens = total_tokens + ?
		WHERE id = 1
	`, successDelta, failureDelta, record.Tokens.TotalTokens); err != nil {
		return false, fmt.Errorf("usage statistics: update summary: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO usage_api_rollups (api_name, total_requests, total_tokens)
		VALUES (?, 1, ?)
		ON CONFLICT(api_name) DO UPDATE SET
			total_requests = total_requests + 1,
			total_tokens = total_tokens + excluded.total_tokens
	`, record.APIName, record.Tokens.TotalTokens); err != nil {
		return false, fmt.Errorf("usage statistics: update api rollup: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO usage_model_rollups (
			api_name, model_name, total_requests, total_tokens,
			input_tokens, output_tokens, reasoning_tokens, cached_tokens
		) VALUES (?, ?, 1, ?, ?, ?, ?, ?)
		ON CONFLICT(api_name, model_name) DO UPDATE SET
			total_requests = total_requests + 1,
			total_tokens = total_tokens + excluded.total_tokens,
			input_tokens = input_tokens + excluded.input_tokens,
			output_tokens = output_tokens + excluded.output_tokens,
			reasoning_tokens = reasoning_tokens + excluded.reasoning_tokens,
			cached_tokens = cached_tokens + excluded.cached_tokens
	`, record.APIName, record.ModelName, record.Tokens.TotalTokens,
		record.Tokens.InputTokens, record.Tokens.OutputTokens, record.Tokens.ReasoningTokens, record.Tokens.CachedTokens); err != nil {
		return false, fmt.Errorf("usage statistics: update model rollup: %w", err)
	}
	dayKey := record.Timestamp.UTC().Format("2006-01-02")
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO usage_daily_rollups (day_key, request_count, total_tokens)
		VALUES (?, 1, ?)
		ON CONFLICT(day_key) DO UPDATE SET
			request_count = request_count + 1,
			total_tokens = total_tokens + excluded.total_tokens
	`, dayKey, record.Tokens.TotalTokens); err != nil {
		return false, fmt.Errorf("usage statistics: update daily rollup: %w", err)
	}
	hourKey := formatHour(record.Timestamp.UTC().Hour())
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO usage_hourly_rollups (hour_key, request_count, total_tokens)
		VALUES (?, 1, ?)
		ON CONFLICT(hour_key) DO UPDATE SET
			request_count = request_count + 1,
			total_tokens = total_tokens + excluded.total_tokens
	`, hourKey, record.Tokens.TotalTokens); err != nil {
		return false, fmt.Errorf("usage statistics: update hourly rollup: %w", err)
	}
	return true, nil
}

func querySummary(ctx context.Context, db *sql.DB, snapshot *StatisticsSnapshot) error {
	if snapshot == nil {
		return nil
	}
	row := db.QueryRowContext(ctx, `
		SELECT total_requests, success_count, failure_count, total_tokens
		FROM usage_summary
		WHERE id = 1
	`)
	if err := row.Scan(&snapshot.TotalRequests, &snapshot.SuccessCount, &snapshot.FailureCount, &snapshot.TotalTokens); err != nil {
		return fmt.Errorf("usage statistics: query summary: %w", err)
	}
	return nil
}

func queryAPIRollups(ctx context.Context, db *sql.DB, snapshot *StatisticsSnapshot) error {
	rows, err := db.QueryContext(ctx, `
		SELECT api_name, total_requests, total_tokens
		FROM usage_api_rollups
		ORDER BY api_name ASC
	`)
	if err != nil {
		return fmt.Errorf("usage statistics: query api rollups: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			apiName       string
			totalRequests int64
			totalTokens   int64
		)
		if err := rows.Scan(&apiName, &totalRequests, &totalTokens); err != nil {
			return fmt.Errorf("usage statistics: scan api rollup: %w", err)
		}
		snapshot.APIs[apiName] = APISnapshot{
			TotalRequests: totalRequests,
			TotalTokens:   totalTokens,
			Models:        make(map[string]ModelSnapshot),
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("usage statistics: iterate api rollups: %w", err)
	}
	return nil
}

func queryModelRollups(ctx context.Context, db *sql.DB, snapshot *StatisticsSnapshot) error {
	rows, err := db.QueryContext(ctx, `
		SELECT api_name, model_name, total_requests, total_tokens, input_tokens, output_tokens, reasoning_tokens, cached_tokens
		FROM usage_model_rollups
		ORDER BY api_name ASC, model_name ASC
	`)
	if err != nil {
		return fmt.Errorf("usage statistics: query model rollups: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			apiName       string
			modelName     string
			modelSnapshot ModelSnapshot
		)
		if err := rows.Scan(
			&apiName,
			&modelName,
			&modelSnapshot.TotalRequests,
			&modelSnapshot.TotalTokens,
			&modelSnapshot.InputTokens,
			&modelSnapshot.OutputTokens,
			&modelSnapshot.ReasoningTokens,
			&modelSnapshot.CachedTokens,
		); err != nil {
			return fmt.Errorf("usage statistics: scan model rollup: %w", err)
		}
		apiSnapshot := snapshot.APIs[apiName]
		if apiSnapshot.Models == nil {
			apiSnapshot.Models = make(map[string]ModelSnapshot)
		}
		apiSnapshot.Models[modelName] = modelSnapshot
		snapshot.APIs[apiName] = apiSnapshot
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("usage statistics: iterate model rollups: %w", err)
	}
	return nil
}

func queryModelDetails(ctx context.Context, db *sql.DB, snapshot *StatisticsSnapshot, options SnapshotOptions) error {
	filter := buildUsageRecordFilter(options.Since)
	args := append([]any{}, filter.args...)
	query := `
		SELECT api_name, model_name, timestamp_utc, source, auth_index, failed,
		       input_tokens, output_tokens, reasoning_tokens, cached_tokens, total_tokens
		FROM usage_records
	`
	if filter.whereClause != "" {
		query += "\n" + filter.whereClause + "\n"
	}
	if options.DetailLimit > 0 {
		query = `
			SELECT api_name, model_name, timestamp_utc, source, auth_index, failed,
			       input_tokens, output_tokens, reasoning_tokens, cached_tokens, total_tokens
			FROM (
				SELECT api_name, model_name, timestamp_utc, source, auth_index, failed,
				       input_tokens, output_tokens, reasoning_tokens, cached_tokens, total_tokens
				FROM usage_records
		` + func() string {
			if filter.whereClause != "" {
				return "\n" + filter.whereClause + "\n"
			}
			return "\n"
		}() + `
				ORDER BY julianday(timestamp_utc) DESC, api_name ASC, model_name ASC
				LIMIT ?
			)
			ORDER BY api_name ASC, model_name ASC, julianday(timestamp_utc) ASC
		`
		args = append(args, options.DetailLimit)
	} else {
		query += "ORDER BY api_name ASC, model_name ASC, julianday(timestamp_utc) ASC\n"
	}
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("usage statistics: query model details: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			apiName      string
			modelName    string
			timestampUTC string
			detail       RequestDetail
			failedInt    int64
		)
		if err := rows.Scan(
			&apiName,
			&modelName,
			&timestampUTC,
			&detail.Source,
			&detail.AuthIndex,
			&failedInt,
			&detail.Tokens.InputTokens,
			&detail.Tokens.OutputTokens,
			&detail.Tokens.ReasoningTokens,
			&detail.Tokens.CachedTokens,
			&detail.Tokens.TotalTokens,
		); err != nil {
			return fmt.Errorf("usage statistics: scan model detail: %w", err)
		}
		parsedTime, err := time.Parse(time.RFC3339Nano, timestampUTC)
		if err != nil {
			return fmt.Errorf("usage statistics: parse model detail timestamp: %w", err)
		}
		detail.Timestamp = parsedTime.UTC()
		detail.Failed = failedInt != 0
		detail.Tokens = normaliseTokenStats(detail.Tokens)

		apiSnapshot := snapshot.APIs[apiName]
		if apiSnapshot.Models == nil {
			apiSnapshot.Models = make(map[string]ModelSnapshot)
		}
		modelSnapshot := apiSnapshot.Models[modelName]
		modelSnapshot.Details = append(modelSnapshot.Details, detail)
		apiSnapshot.Models[modelName] = modelSnapshot
		snapshot.APIs[apiName] = apiSnapshot
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("usage statistics: iterate model details: %w", err)
	}
	return nil
}

type usageRecordFilter struct {
	whereClause string
	args        []any
}

func newStatisticsSnapshot() StatisticsSnapshot {
	return StatisticsSnapshot{
		APIs:           make(map[string]APISnapshot),
		RequestsByDay:  make(map[string]int64),
		RequestsByHour: make(map[string]int64),
		TokensByDay:    make(map[string]int64),
		TokensByHour:   make(map[string]int64),
	}
}

func normalizeSnapshotOptions(options SnapshotOptions) SnapshotOptions {
	if !options.Since.IsZero() {
		options.Since = options.Since.UTC()
	}
	if options.DetailLimit < 0 {
		options.DetailLimit = 0
	}
	return options
}

func buildUsageRecordFilter(since time.Time) usageRecordFilter {
	if since.IsZero() {
		return usageRecordFilter{}
	}
	return usageRecordFilter{
		whereClause: "WHERE julianday(timestamp_utc) >= julianday(?)",
		args:        []any{since.UTC().Format(time.RFC3339Nano)},
	}
}

func querySummaryFromRecords(ctx context.Context, db *sql.DB, snapshot *StatisticsSnapshot, filter usageRecordFilter) error {
	if snapshot == nil {
		return nil
	}
	query := `
		SELECT COUNT(*),
		       COALESCE(SUM(CASE WHEN failed = 0 THEN 1 ELSE 0 END), 0),
		       COALESCE(SUM(CASE WHEN failed != 0 THEN 1 ELSE 0 END), 0),
		       COALESCE(SUM(total_tokens), 0)
		FROM usage_records
	`
	if filter.whereClause != "" {
		query += "\n" + filter.whereClause + "\n"
	}
	row := db.QueryRowContext(ctx, query, filter.args...)
	if err := row.Scan(&snapshot.TotalRequests, &snapshot.SuccessCount, &snapshot.FailureCount, &snapshot.TotalTokens); err != nil {
		return fmt.Errorf("usage statistics: query filtered summary: %w", err)
	}
	return nil
}

func queryAPIRollupsFromRecords(ctx context.Context, db *sql.DB, snapshot *StatisticsSnapshot, filter usageRecordFilter) error {
	query := `
		SELECT api_name, COUNT(*), COALESCE(SUM(total_tokens), 0)
		FROM usage_records
	`
	if filter.whereClause != "" {
		query += "\n" + filter.whereClause + "\n"
	}
	query += "GROUP BY api_name ORDER BY api_name ASC"
	rows, err := db.QueryContext(ctx, query, filter.args...)
	if err != nil {
		return fmt.Errorf("usage statistics: query filtered api rollups: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			apiName       string
			totalRequests int64
			totalTokens   int64
		)
		if err := rows.Scan(&apiName, &totalRequests, &totalTokens); err != nil {
			return fmt.Errorf("usage statistics: scan filtered api rollup: %w", err)
		}
		snapshot.APIs[apiName] = APISnapshot{
			TotalRequests: totalRequests,
			TotalTokens:   totalTokens,
			Models:        make(map[string]ModelSnapshot),
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("usage statistics: iterate filtered api rollups: %w", err)
	}
	return nil
}

func queryModelRollupsFromRecords(ctx context.Context, db *sql.DB, snapshot *StatisticsSnapshot, filter usageRecordFilter) error {
	query := `
		SELECT api_name, model_name, COUNT(*), COALESCE(SUM(total_tokens), 0),
		       COALESCE(SUM(input_tokens), 0), COALESCE(SUM(output_tokens), 0),
		       COALESCE(SUM(reasoning_tokens), 0), COALESCE(SUM(cached_tokens), 0)
		FROM usage_records
	`
	if filter.whereClause != "" {
		query += "\n" + filter.whereClause + "\n"
	}
	query += "GROUP BY api_name, model_name ORDER BY api_name ASC, model_name ASC"
	rows, err := db.QueryContext(ctx, query, filter.args...)
	if err != nil {
		return fmt.Errorf("usage statistics: query filtered model rollups: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			apiName       string
			modelName     string
			modelSnapshot ModelSnapshot
		)
		if err := rows.Scan(
			&apiName,
			&modelName,
			&modelSnapshot.TotalRequests,
			&modelSnapshot.TotalTokens,
			&modelSnapshot.InputTokens,
			&modelSnapshot.OutputTokens,
			&modelSnapshot.ReasoningTokens,
			&modelSnapshot.CachedTokens,
		); err != nil {
			return fmt.Errorf("usage statistics: scan filtered model rollup: %w", err)
		}
		apiSnapshot := snapshot.APIs[apiName]
		if apiSnapshot.Models == nil {
			apiSnapshot.Models = make(map[string]ModelSnapshot)
		}
		apiSnapshot.Models[modelName] = modelSnapshot
		snapshot.APIs[apiName] = apiSnapshot
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("usage statistics: iterate filtered model rollups: %w", err)
	}
	return nil
}

func queryDailyRollupsFromRecords(ctx context.Context, db *sql.DB, snapshot *StatisticsSnapshot, filter usageRecordFilter) error {
	query := `
		SELECT date(timestamp_utc), COUNT(*), COALESCE(SUM(total_tokens), 0)
		FROM usage_records
	`
	if filter.whereClause != "" {
		query += "\n" + filter.whereClause + "\n"
	}
	query += "GROUP BY date(timestamp_utc) ORDER BY date(timestamp_utc) ASC"
	rows, err := db.QueryContext(ctx, query, filter.args...)
	if err != nil {
		return fmt.Errorf("usage statistics: query filtered daily rollups: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			dayKey       string
			requestCount int64
			totalTokens  int64
		)
		if err := rows.Scan(&dayKey, &requestCount, &totalTokens); err != nil {
			return fmt.Errorf("usage statistics: scan filtered daily rollup: %w", err)
		}
		snapshot.RequestsByDay[dayKey] = requestCount
		snapshot.TokensByDay[dayKey] = totalTokens
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("usage statistics: iterate filtered daily rollups: %w", err)
	}
	return nil
}

func queryHourlyRollupsFromRecords(ctx context.Context, db *sql.DB, snapshot *StatisticsSnapshot, filter usageRecordFilter) error {
	query := `
		SELECT strftime('%H', timestamp_utc), COUNT(*), COALESCE(SUM(total_tokens), 0)
		FROM usage_records
	`
	if filter.whereClause != "" {
		query += "\n" + filter.whereClause + "\n"
	}
	query += "GROUP BY strftime('%H', timestamp_utc) ORDER BY strftime('%H', timestamp_utc) ASC"
	rows, err := db.QueryContext(ctx, query, filter.args...)
	if err != nil {
		return fmt.Errorf("usage statistics: query filtered hourly rollups: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			hourKey      string
			requestCount int64
			totalTokens  int64
		)
		if err := rows.Scan(&hourKey, &requestCount, &totalTokens); err != nil {
			return fmt.Errorf("usage statistics: scan filtered hourly rollup: %w", err)
		}
		snapshot.RequestsByHour[hourKey] = requestCount
		snapshot.TokensByHour[hourKey] = totalTokens
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("usage statistics: iterate filtered hourly rollups: %w", err)
	}
	return nil
}

func queryDailyRollups(ctx context.Context, db *sql.DB, snapshot *StatisticsSnapshot) error {
	rows, err := db.QueryContext(ctx, `
		SELECT day_key, request_count, total_tokens
		FROM usage_daily_rollups
		ORDER BY day_key ASC
	`)
	if err != nil {
		return fmt.Errorf("usage statistics: query daily rollups: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			dayKey       string
			requestCount int64
			totalTokens  int64
		)
		if err := rows.Scan(&dayKey, &requestCount, &totalTokens); err != nil {
			return fmt.Errorf("usage statistics: scan daily rollup: %w", err)
		}
		snapshot.RequestsByDay[dayKey] = requestCount
		snapshot.TokensByDay[dayKey] = totalTokens
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("usage statistics: iterate daily rollups: %w", err)
	}
	return nil
}

func queryHourlyRollups(ctx context.Context, db *sql.DB, snapshot *StatisticsSnapshot) error {
	rows, err := db.QueryContext(ctx, `
		SELECT hour_key, request_count, total_tokens
		FROM usage_hourly_rollups
		ORDER BY hour_key ASC
	`)
	if err != nil {
		return fmt.Errorf("usage statistics: query hourly rollups: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			hourKey      string
			requestCount int64
			totalTokens  int64
		)
		if err := rows.Scan(&hourKey, &requestCount, &totalTokens); err != nil {
			return fmt.Errorf("usage statistics: scan hourly rollup: %w", err)
		}
		snapshot.RequestsByHour[hourKey] = requestCount
		snapshot.TokensByHour[hourKey] = totalTokens
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("usage statistics: iterate hourly rollups: %w", err)
	}
	return nil
}
