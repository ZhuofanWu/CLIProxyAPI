package usage

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	coreusage "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/usage"
)

const (
	UsageStorageWayMemory = "memory"
	UsageStorageWaySQLite = "sqlite"

	usageDatabaseFileName = "usage.db"
	usageExportVersionV2  = 2
)

type statisticsStore interface {
	Record(context.Context, coreusage.Record)
	Snapshot() StatisticsSnapshot
	SnapshotContext(context.Context) (StatisticsSnapshot, error)
	SnapshotContextWithOptions(context.Context, SnapshotOptions) (StatisticsSnapshot, error)
	ExportRecords(context.Context) ([]PersistedRecord, error)
	MergeRecords(context.Context, []PersistedRecord) (MergeResult, error)
	MergeSnapshot(StatisticsSnapshot) MergeResult
	MergeSnapshotContext(context.Context, StatisticsSnapshot) (MergeResult, error)
	Configure(string) error
	DatabasePath() string
	Close() error
}

// RequestStatistics routes usage reads and writes to the configured storage backend.
type RequestStatistics struct {
	mu         sync.RWMutex
	storageWay string
	store      statisticsStore
}

// NewRequestStatistics constructs a usage store using the legacy in-memory backend.
func NewRequestStatistics() *RequestStatistics {
	return &RequestStatistics{
		storageWay: UsageStorageWayMemory,
		store:      newMemoryStore(),
	}
}

// NormalizeUsageStorageWay normalizes the configured storage backend selector.
func NormalizeUsageStorageWay(raw string) (string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	switch normalized {
	case "", UsageStorageWayMemory:
		return UsageStorageWayMemory, true
	case UsageStorageWaySQLite:
		return UsageStorageWaySQLite, true
	default:
		return "", false
	}
}

// StorageWay reports the active storage backend.
func (s *RequestStatistics) StorageWay() string {
	if s == nil {
		return UsageStorageWayMemory
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.storageWay == "" {
		return UsageStorageWayMemory
	}
	return s.storageWay
}

// Configure switches this usage store to SQLite persistence under authDir.
func (s *RequestStatistics) Configure(authDir string) error {
	return s.ConfigureStorageWay(UsageStorageWaySQLite, authDir)
}

// ConfigureStorageWay switches the underlying storage backend while preserving usage data.
func (s *RequestStatistics) ConfigureStorageWay(rawWay, authDir string) error {
	if s == nil {
		return nil
	}
	storageWay, ok := NormalizeUsageStorageWay(rawWay)
	if !ok {
		return fmt.Errorf("invalid usage storage way: %q", rawWay)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.store == nil {
		s.storageWay = UsageStorageWayMemory
		s.store = newMemoryStore()
	}

	if s.storageWay == storageWay {
		if storageWay == UsageStorageWaySQLite {
			return s.store.Configure(authDir)
		}
		return nil
	}

	currentStore := s.store
	snapshot, err := currentStore.SnapshotContext(context.Background())
	if err != nil {
		return err
	}

	nextStore := newStatisticsStore(storageWay)
	if storageWay == UsageStorageWaySQLite {
		if err := nextStore.Configure(authDir); err != nil {
			return err
		}
	}
	if _, err := nextStore.MergeSnapshotContext(context.Background(), snapshot); err != nil {
		_ = nextStore.Close()
		return err
	}

	s.store = nextStore
	s.storageWay = storageWay
	if currentStore != nil {
		return currentStore.Close()
	}
	return nil
}

// DatabasePath returns the configured SQLite file path when SQLite mode is active.
func (s *RequestStatistics) DatabasePath() string {
	if s == nil {
		return ""
	}
	store := s.currentStore()
	if store == nil {
		return ""
	}
	return store.DatabasePath()
}

// Close closes the current storage backend.
func (s *RequestStatistics) Close() error {
	if s == nil {
		return nil
	}
	store := s.currentStore()
	if store == nil {
		return nil
	}
	return store.Close()
}

// Record ingests a new usage record.
func (s *RequestStatistics) Record(ctx context.Context, record coreusage.Record) {
	if s == nil {
		return
	}
	store := s.currentStore()
	if store == nil {
		return
	}
	store.Record(ctx, record)
}

// Snapshot returns a best-effort snapshot of usage statistics.
func (s *RequestStatistics) Snapshot() StatisticsSnapshot {
	if s == nil {
		return StatisticsSnapshot{}
	}
	store := s.currentStore()
	if store == nil {
		return StatisticsSnapshot{}
	}
	return store.Snapshot()
}

// SnapshotContext returns a snapshot of usage statistics.
func (s *RequestStatistics) SnapshotContext(ctx context.Context) (StatisticsSnapshot, error) {
	if s == nil {
		return StatisticsSnapshot{}, nil
	}
	store := s.currentStore()
	if store == nil {
		return StatisticsSnapshot{}, nil
	}
	return store.SnapshotContext(ctx)
}

// SnapshotContextWithOptions returns a snapshot of usage statistics with backend-specific options.
func (s *RequestStatistics) SnapshotContextWithOptions(ctx context.Context, options SnapshotOptions) (StatisticsSnapshot, error) {
	if s == nil {
		return StatisticsSnapshot{}, nil
	}
	store := s.currentStore()
	if store == nil {
		return StatisticsSnapshot{}, nil
	}
	return store.SnapshotContextWithOptions(ctx, options)
}

// ExportRecords exports persisted records when supported by the current backend.
func (s *RequestStatistics) ExportRecords(ctx context.Context) ([]PersistedRecord, error) {
	if s == nil {
		return nil, nil
	}
	store := s.currentStore()
	if store == nil {
		return nil, nil
	}
	return store.ExportRecords(ctx)
}

// MergeRecords imports persisted records into the current backend.
func (s *RequestStatistics) MergeRecords(ctx context.Context, records []PersistedRecord) (MergeResult, error) {
	if s == nil {
		return MergeResult{}, nil
	}
	store := s.currentStore()
	if store == nil {
		return MergeResult{}, nil
	}
	return store.MergeRecords(ctx, records)
}

// MergeSnapshot merges a legacy snapshot into the current backend.
func (s *RequestStatistics) MergeSnapshot(snapshot StatisticsSnapshot) MergeResult {
	if s == nil {
		return MergeResult{}
	}
	store := s.currentStore()
	if store == nil {
		return MergeResult{}
	}
	return store.MergeSnapshot(snapshot)
}

// MergeSnapshotContext merges a legacy snapshot into the current backend.
func (s *RequestStatistics) MergeSnapshotContext(ctx context.Context, snapshot StatisticsSnapshot) (MergeResult, error) {
	if s == nil {
		return MergeResult{}, nil
	}
	store := s.currentStore()
	if store == nil {
		return MergeResult{}, nil
	}
	return store.MergeSnapshotContext(ctx, snapshot)
}

func (s *RequestStatistics) currentStore() statisticsStore {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.store
}

func newStatisticsStore(storageWay string) statisticsStore {
	if storageWay == UsageStorageWaySQLite {
		return &sqliteStore{}
	}
	return newMemoryStore()
}

// RequestDetail stores the timestamp and token usage for a single request.
type RequestDetail struct {
	Timestamp time.Time  `json:"timestamp"`
	Source    string     `json:"source"`
	AuthIndex string     `json:"auth_index"`
	Tokens    TokenStats `json:"tokens"`
	Failed    bool       `json:"failed"`
}

// TokenStats captures the token usage breakdown for a request.
type TokenStats struct {
	InputTokens     int64 `json:"input_tokens"`
	OutputTokens    int64 `json:"output_tokens"`
	ReasoningTokens int64 `json:"reasoning_tokens"`
	CachedTokens    int64 `json:"cached_tokens"`
	TotalTokens     int64 `json:"total_tokens"`
}

// StatisticsSnapshot represents an immutable view of the aggregated metrics.
type StatisticsSnapshot struct {
	TotalRequests int64 `json:"total_requests"`
	SuccessCount  int64 `json:"success_count"`
	FailureCount  int64 `json:"failure_count"`
	TotalTokens   int64 `json:"total_tokens"`

	APIs map[string]APISnapshot `json:"apis"`

	RequestsByDay  map[string]int64 `json:"requests_by_day"`
	RequestsByHour map[string]int64 `json:"requests_by_hour"`
	TokensByDay    map[string]int64 `json:"tokens_by_day"`
	TokensByHour   map[string]int64 `json:"tokens_by_hour"`
}

// APISnapshot summarises metrics for a single API key.
type APISnapshot struct {
	TotalRequests int64                    `json:"total_requests"`
	TotalTokens   int64                    `json:"total_tokens"`
	Models        map[string]ModelSnapshot `json:"models"`
}

// ModelSnapshot summarises metrics for a specific model.
type ModelSnapshot struct {
	TotalRequests   int64           `json:"total_requests"`
	TotalTokens     int64           `json:"total_tokens"`
	InputTokens     int64           `json:"input_tokens,omitempty"`
	OutputTokens    int64           `json:"output_tokens,omitempty"`
	ReasoningTokens int64           `json:"reasoning_tokens,omitempty"`
	CachedTokens    int64           `json:"cached_tokens,omitempty"`
	Details         []RequestDetail `json:"details"`
}

// PersistedRecord is the durable representation used for export/import.
type PersistedRecord struct {
	APIName   string     `json:"api_name"`
	ModelName string     `json:"model_name"`
	Timestamp time.Time  `json:"timestamp"`
	Source    string     `json:"source"`
	AuthIndex string     `json:"auth_index"`
	Failed    bool       `json:"failed"`
	Tokens    TokenStats `json:"tokens"`
}

// MergeResult reports how many records were imported or skipped.
type MergeResult struct {
	Added   int64 `json:"added"`
	Skipped int64 `json:"skipped"`
}

// SnapshotOptions controls how usage snapshots are materialized.
type SnapshotOptions struct {
	Since       time.Time
	DetailLimit int
}

func resolveAPIIdentifier(ctx context.Context, record coreusage.Record) string {
	if ctx != nil {
		if ginCtx, ok := ctx.Value("gin").(*gin.Context); ok && ginCtx != nil {
			path := ginCtx.FullPath()
			if path == "" && ginCtx.Request != nil {
				path = ginCtx.Request.URL.Path
			}
			method := ""
			if ginCtx.Request != nil {
				method = ginCtx.Request.Method
			}
			if path != "" {
				if method != "" {
					return method + " " + path
				}
				return path
			}
		}
	}
	if record.Provider != "" {
		return record.Provider
	}
	return "unknown"
}

func resolveSuccess(ctx context.Context) bool {
	if ctx == nil {
		return true
	}
	ginCtx, ok := ctx.Value("gin").(*gin.Context)
	if !ok || ginCtx == nil {
		return true
	}
	status := ginCtx.Writer.Status()
	if status == 0 {
		return true
	}
	return status < httpStatusBadRequest
}

const httpStatusBadRequest = 400

func normaliseDetail(detail coreusage.Detail) TokenStats {
	tokens := TokenStats{
		InputTokens:     detail.InputTokens,
		OutputTokens:    detail.OutputTokens,
		ReasoningTokens: detail.ReasoningTokens,
		CachedTokens:    detail.CachedTokens,
		TotalTokens:     detail.TotalTokens,
	}
	return normaliseTokenStats(tokens)
}

func normaliseTokenStats(tokens TokenStats) TokenStats {
	if tokens.TotalTokens == 0 {
		tokens.TotalTokens = tokens.InputTokens + tokens.OutputTokens + tokens.ReasoningTokens
	}
	if tokens.TotalTokens == 0 {
		tokens.TotalTokens = tokens.InputTokens + tokens.OutputTokens + tokens.ReasoningTokens + tokens.CachedTokens
	}
	if tokens.TotalTokens < 0 {
		tokens.TotalTokens = 0
	}
	return tokens
}

func formatHour(hour int) string {
	if hour < 0 {
		hour = 0
	}
	hour = hour % 24
	return fmt.Sprintf("%02d", hour)
}

func dedupKey(apiName, modelName string, detail RequestDetail) string {
	timestamp := detail.Timestamp.UTC().Format(time.RFC3339Nano)
	tokens := normaliseTokenStats(detail.Tokens)
	return fmt.Sprintf(
		"%s|%s|%s|%s|%s|%t|%d|%d|%d|%d|%d",
		apiName,
		modelName,
		timestamp,
		detail.Source,
		detail.AuthIndex,
		detail.Failed,
		tokens.InputTokens,
		tokens.OutputTokens,
		tokens.ReasoningTokens,
		tokens.CachedTokens,
		tokens.TotalTokens,
	)
}

func normalizePersistedRecord(record PersistedRecord) PersistedRecord {
	record.APIName = strings.TrimSpace(record.APIName)
	if record.APIName == "" {
		record.APIName = "unknown"
	}
	record.ModelName = strings.TrimSpace(record.ModelName)
	if record.ModelName == "" {
		record.ModelName = "unknown"
	}
	record.Source = strings.TrimSpace(record.Source)
	record.AuthIndex = strings.TrimSpace(record.AuthIndex)
	record.Tokens = normaliseTokenStats(record.Tokens)
	if record.Timestamp.IsZero() {
		record.Timestamp = time.Now().UTC()
	} else {
		record.Timestamp = record.Timestamp.UTC()
	}
	return record
}

func recordsFromSnapshot(snapshot StatisticsSnapshot) []PersistedRecord {
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
	sort.SliceStable(records, func(i, j int) bool {
		left := records[i]
		right := records[j]
		if !left.Timestamp.Equal(right.Timestamp) {
			return left.Timestamp.Before(right.Timestamp)
		}
		if left.APIName != right.APIName {
			return left.APIName < right.APIName
		}
		if left.ModelName != right.ModelName {
			return left.ModelName < right.ModelName
		}
		if left.Source != right.Source {
			return left.Source < right.Source
		}
		return left.AuthIndex < right.AuthIndex
	})
	return records
}

func recordFromUsageRecord(ctx context.Context, record coreusage.Record) PersistedRecord {
	timestamp := record.RequestedAt
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}
	detail := normaliseDetail(record.Detail)
	statsKey := strings.TrimSpace(record.APIKey)
	if statsKey == "" {
		statsKey = resolveAPIIdentifier(ctx, record)
	}
	failed := record.Failed
	if !failed {
		failed = !resolveSuccess(ctx)
	}
	modelName := strings.TrimSpace(record.Model)
	if modelName == "" {
		modelName = "unknown"
	}
	return normalizePersistedRecord(PersistedRecord{
		APIName:   statsKey,
		ModelName: modelName,
		Timestamp: timestamp,
		Source:    record.Source,
		AuthIndex: record.AuthIndex,
		Failed:    failed,
		Tokens:    detail,
	})
}
