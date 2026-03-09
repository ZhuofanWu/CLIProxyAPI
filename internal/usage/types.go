package usage

import (
	"context"
	"errors"
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

var ErrGeneralUnsupported = errors.New("usage general requires sqlite storage")
var ErrHealthUnsupported = errors.New("usage health requires sqlite storage")
var ErrTokenBreakdownUnsupported = errors.New("usage token breakdown requires sqlite storage")
var ErrCostTrendUnsupported = errors.New("usage cost trend requires sqlite storage")
var ErrRankingsUnsupported = errors.New("usage rankings requires sqlite storage")

type statisticsStore interface {
	Record(context.Context, coreusage.Record)
	Snapshot() StatisticsSnapshot
	SnapshotContext(context.Context) (StatisticsSnapshot, error)
	SnapshotContextWithOptions(context.Context, SnapshotOptions) (StatisticsSnapshot, error)
	GeneralContext(context.Context, GeneralOptions) (GeneralSnapshot, error)
	HealthContext(context.Context, HealthOptions) (HealthSnapshot, error)
	TokenBreakdownContext(context.Context, TokenBreakdownOptions) (TokenBreakdownSnapshot, error)
	CostTrendContext(context.Context, CostTrendOptions) (CostTrendSnapshot, error)
	RankingsContext(context.Context, RankingsOptions) (RankingsSnapshot, error)
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

// GeneralContext returns sqlite-only aggregated usage overview data.
func (s *RequestStatistics) GeneralContext(ctx context.Context, options GeneralOptions) (GeneralSnapshot, error) {
	if s == nil {
		return GeneralSnapshot{}, nil
	}
	store := s.currentStore()
	if store == nil {
		return GeneralSnapshot{}, nil
	}
	return store.GeneralContext(ctx, options)
}

// HealthContext returns sqlite-only aggregated service health data.
func (s *RequestStatistics) HealthContext(ctx context.Context, options HealthOptions) (HealthSnapshot, error) {
	if s == nil {
		return HealthSnapshot{}, nil
	}
	store := s.currentStore()
	if store == nil {
		return HealthSnapshot{}, nil
	}
	return store.HealthContext(ctx, options)
}

// TokenBreakdownContext returns sqlite-only token breakdown buckets for the usage page.
func (s *RequestStatistics) TokenBreakdownContext(
	ctx context.Context,
	options TokenBreakdownOptions,
) (TokenBreakdownSnapshot, error) {
	if s == nil {
		return TokenBreakdownSnapshot{}, nil
	}
	store := s.currentStore()
	if store == nil {
		return TokenBreakdownSnapshot{}, nil
	}
	return store.TokenBreakdownContext(ctx, options)
}

// CostTrendContext returns sqlite-only cost trend buckets for the usage page.
func (s *RequestStatistics) CostTrendContext(
	ctx context.Context,
	options CostTrendOptions,
) (CostTrendSnapshot, error) {
	if s == nil {
		return CostTrendSnapshot{}, nil
	}
	store := s.currentStore()
	if store == nil {
		return CostTrendSnapshot{}, nil
	}
	return store.CostTrendContext(ctx, options)
}

// RankingsContext returns sqlite-only aggregated ranking data for the usage page.
func (s *RequestStatistics) RankingsContext(
	ctx context.Context,
	options RankingsOptions,
) (RankingsSnapshot, error) {
	if s == nil {
		return RankingsSnapshot{}, nil
	}
	store := s.currentStore()
	if store == nil {
		return RankingsSnapshot{}, nil
	}
	return store.RankingsContext(ctx, options)
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

// ModelPrice captures backend pricing used by sqlite-only usage aggregations.
type ModelPrice struct {
	PromptMilli     int64
	CompletionMilli int64
	CacheMilli      int64
}

// GeneralOptions controls how sqlite usage general statistics are materialized.
type GeneralOptions struct {
	Since       time.Time
	Now         time.Time
	ModelPrices map[string]ModelPrice
}

// GeneralPoint is a timestamped numeric point for usage overview sparklines.
type GeneralPoint struct {
	Timestamp time.Time `json:"ts"`
	Value     float64   `json:"value"`
}

// GeneralSummary contains the overview card values and supporting metadata.
type GeneralSummary struct {
	TotalRequests      int64   `json:"total_requests"`
	SuccessCount       int64   `json:"success_count"`
	FailureCount       int64   `json:"failure_count"`
	TotalTokens        int64   `json:"total_tokens"`
	CachedTokens       int64   `json:"cached_tokens"`
	ReasoningTokens    int64   `json:"reasoning_tokens"`
	RPM30m             float64 `json:"rpm_30m"`
	RPMRequestCount30m int64   `json:"rpm_request_count_30m"`
	TPM30m             float64 `json:"tpm_30m"`
	TPMTokenCount30m   int64   `json:"tpm_token_count_30m"`
	TotalCost          float64 `json:"total_cost"`
	CostAvailable      bool    `json:"cost_available"`
}

// GeneralSeries contains sqlite-only minute-level series for the usage overview cards.
type GeneralSeries struct {
	Requests60m []GeneralPoint `json:"requests_60m"`
	Tokens60m   []GeneralPoint `json:"tokens_60m"`
	RPM30m      []GeneralPoint `json:"rpm_30m"`
	TPM30m      []GeneralPoint `json:"tpm_30m"`
	Cost30m     []GeneralPoint `json:"cost_30m"`
}

// GeneralSnapshot is the sqlite-only payload returned by /usage/general.
type GeneralSnapshot struct {
	Summary GeneralSummary `json:"summary"`
	Series  GeneralSeries  `json:"series"`
}

// HealthOptions controls how sqlite usage health statistics are materialized.
type HealthOptions struct {
	Now time.Time
}

// HealthSnapshot is the sqlite-only payload returned by /usage/health.
type HealthSnapshot struct {
	Rates         []int     `json:"rates"`
	SuccessCounts []int64   `json:"success_counts"`
	FailureCounts []int64   `json:"failure_counts"`
	WindowStart   time.Time `json:"window_start"`
	WindowEnd     time.Time `json:"window_end"`
	BucketMinutes int       `json:"bucket_minutes"`
	Rows          int       `json:"rows"`
	Cols          int       `json:"cols"`
	SuccessRate   float64   `json:"success_rate"`
	TotalSuccess  int64     `json:"total_success"`
	TotalFailure  int64     `json:"total_failure"`
}

// TokenBreakdownOptions controls how sqlite token breakdown buckets are materialized.
type TokenBreakdownOptions struct {
	Granularity string
	Range       string
	Offset      int
	Now         time.Time
}

// TokenBreakdownBucket contains one bucket of aggregated token usage.
type TokenBreakdownBucket struct {
	Label           string `json:"label"`
	InputTokens     int64  `json:"input_tokens"`
	OutputTokens    int64  `json:"output_tokens"`
	CachedTokens    int64  `json:"cached_tokens"`
	ReasoningTokens int64  `json:"reasoning_tokens"`
}

// TokenBreakdownSnapshot is the sqlite-only payload returned by /usage/token-breakdown.
type TokenBreakdownSnapshot struct {
	Granularity string                 `json:"granularity"`
	Range       string                 `json:"range"`
	Offset      int                    `json:"offset"`
	HasOlder    bool                   `json:"has_older"`
	Buckets     []TokenBreakdownBucket `json:"buckets"`
}

// CostTrendOptions controls how sqlite cost trend buckets are materialized.
type CostTrendOptions struct {
	Granularity string
	Range       string
	Offset      int
	Now         time.Time
	ModelPrices map[string]ModelPrice
}

// CostTrendBucket contains one bucket of aggregated cost.
type CostTrendBucket struct {
	Label string  `json:"label"`
	Cost  float64 `json:"cost"`
}

// CostTrendSnapshot is the sqlite-only payload returned by /usage/cost-trend.
type CostTrendSnapshot struct {
	Granularity string            `json:"granularity"`
	Range       string            `json:"range"`
	Offset      int               `json:"offset"`
	HasOlder    bool              `json:"has_older"`
	Buckets     []CostTrendBucket `json:"buckets"`
}

// RankingsOptions controls how sqlite usage ranking data is materialized.
type RankingsOptions struct {
	Since       time.Time
	Now         time.Time
	ModelPrices map[string]ModelPrice
}

// APIRankingModel captures one model row nested under an API ranking item.
type APIRankingModel struct {
	ModelName    string `json:"model_name"`
	Requests     int64  `json:"requests"`
	SuccessCount int64  `json:"success_count"`
	FailureCount int64  `json:"failure_count"`
	Tokens       int64  `json:"tokens"`
}

// APIRanking contains aggregated data for the API details ranking card.
type APIRanking struct {
	APIName       string            `json:"api_name"`
	TotalRequests int64             `json:"total_requests"`
	SuccessCount  int64             `json:"success_count"`
	FailureCount  int64             `json:"failure_count"`
	TotalTokens   int64             `json:"total_tokens"`
	TotalCost     float64           `json:"total_cost"`
	Models        []APIRankingModel `json:"models"`
}

// ModelRanking contains aggregated data for the model statistics ranking card.
type ModelRanking struct {
	ModelName    string  `json:"model_name"`
	Requests     int64   `json:"requests"`
	SuccessCount int64   `json:"success_count"`
	FailureCount int64   `json:"failure_count"`
	Tokens       int64   `json:"tokens"`
	Cost         float64 `json:"cost"`
}

// RankingsSnapshot is the sqlite-only payload returned by /usage/rankings.
type RankingsSnapshot struct {
	APIRankings   []APIRanking   `json:"api_rankings"`
	ModelRankings []ModelRanking `json:"model_rankings"`
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
