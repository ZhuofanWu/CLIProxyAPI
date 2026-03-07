package usage

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	coreusage "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/usage"
)

const (
	usageDatabaseFileName = "usage.db"
	usageExportVersionV2  = 2
)

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
	InputTokens     int64           `json:"input_tokens"`
	OutputTokens    int64           `json:"output_tokens"`
	ReasoningTokens int64           `json:"reasoning_tokens"`
	CachedTokens    int64           `json:"cached_tokens"`
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
