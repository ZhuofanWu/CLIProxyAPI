package management

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/usage"
	coreusage "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/usage"
)

func TestGetUsageStatistics_MemoryIgnoresRangeAndReturnsFullDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	original := usage.StatisticsEnabled()
	usage.SetStatisticsEnabled(true)
	t.Cleanup(func() { usage.SetStatisticsEnabled(original) })

	stats := usage.NewRequestStatistics()
	stats.Record(context.Background(), coreusage.Record{
		APIKey:      "api-memory",
		Model:       "model-a",
		RequestedAt: time.Now().UTC().Add(-10 * 24 * time.Hour),
		Detail: coreusage.Detail{
			InputTokens:  1,
			OutputTokens: 2,
			TotalTokens:  3,
		},
	})
	stats.Record(context.Background(), coreusage.Record{
		APIKey:      "api-memory",
		Model:       "model-a",
		RequestedAt: time.Now().UTC(),
		Detail: coreusage.Detail{
			InputTokens:  4,
			OutputTokens: 5,
			TotalTokens:  9,
		},
	})

	h := &Handler{usageStats: stats}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v0/management/usage?range=7h", nil)

	h.GetUsageStatistics(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var payload map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	usageMap := payload["usage"].(map[string]any)
	if got := int64(usageMap["total_requests"].(float64)); got != 2 {
		t.Fatalf("total_requests = %d, want 2", got)
	}
	apis := usageMap["apis"].(map[string]any)
	models := apis["api-memory"].(map[string]any)["models"].(map[string]any)
	modelSnapshot := models["model-a"].(map[string]any)
	if got := int64(modelSnapshot["input_tokens"].(float64)); got != 5 {
		t.Fatalf("input_tokens = %d, want 5", got)
	}
	if got := int64(modelSnapshot["output_tokens"].(float64)); got != 7 {
		t.Fatalf("output_tokens = %d, want 7", got)
	}
	details := modelSnapshot["details"].([]any)
	if len(details) != 2 {
		t.Fatalf("details len = %d, want 2", len(details))
	}
}

func TestGetUsageStatistics_SQLiteIgnoresRangeAndReturnsFullDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	original := usage.StatisticsEnabled()
	usage.SetStatisticsEnabled(true)
	t.Cleanup(func() { usage.SetStatisticsEnabled(original) })

	stats := usage.NewRequestStatistics()
	if err := stats.Configure(t.TempDir()); err != nil {
		t.Fatalf("Configure sqlite stats: %v", err)
	}
	t.Cleanup(func() { _ = stats.Close() })

	stats.Record(context.Background(), coreusage.Record{
		APIKey:      "api-sqlite",
		Model:       "model-a",
		RequestedAt: time.Now().UTC().Add(-10 * 24 * time.Hour),
		Detail: coreusage.Detail{
			InputTokens:  1,
			OutputTokens: 2,
			TotalTokens:  3,
		},
	})
	stats.Record(context.Background(), coreusage.Record{
		APIKey:      "api-sqlite",
		Model:       "model-a",
		RequestedAt: time.Now().UTC(),
		Detail: coreusage.Detail{
			InputTokens:     4,
			OutputTokens:    5,
			ReasoningTokens: 1,
			CachedTokens:    2,
			TotalTokens:     12,
		},
	})

	h := &Handler{usageStats: stats}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v0/management/usage?range=not-used-anymore", nil)

	h.GetUsageStatistics(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var payload map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	usageMap := payload["usage"].(map[string]any)
	if got := int64(usageMap["total_requests"].(float64)); got != 2 {
		t.Fatalf("total_requests = %d, want 2", got)
	}
	apis := usageMap["apis"].(map[string]any)
	models := apis["api-sqlite"].(map[string]any)["models"].(map[string]any)
	modelSnapshot := models["model-a"].(map[string]any)
	if got := int64(modelSnapshot["input_tokens"].(float64)); got != 5 {
		t.Fatalf("input_tokens = %d, want 5", got)
	}
	if got := int64(modelSnapshot["output_tokens"].(float64)); got != 7 {
		t.Fatalf("output_tokens = %d, want 7", got)
	}
	if got := int64(modelSnapshot["reasoning_tokens"].(float64)); got != 1 {
		t.Fatalf("reasoning_tokens = %d, want 1", got)
	}
	if got := int64(modelSnapshot["cached_tokens"].(float64)); got != 2 {
		t.Fatalf("cached_tokens = %d, want 2", got)
	}
	details := modelSnapshot["details"].([]any)
	if len(details) != 2 {
		t.Fatalf("details len = %d, want 2", len(details))
	}
}

func TestUsageExportImport_MemoryKeepsLegacyVersioning(t *testing.T) {
	gin.SetMode(gin.TestMode)
	original := usage.StatisticsEnabled()
	usage.SetStatisticsEnabled(true)
	t.Cleanup(func() { usage.SetStatisticsEnabled(original) })

	stats := usage.NewRequestStatistics()
	stats.Record(context.Background(), coreusage.Record{
		APIKey:      "api-memory",
		Model:       "model-a",
		RequestedAt: time.Now().UTC(),
		Detail: coreusage.Detail{
			InputTokens:  1,
			OutputTokens: 2,
			TotalTokens:  3,
		},
	})

	h := &Handler{usageStats: stats}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v0/management/usage/export", nil)
	h.ExportUsageStatistics(c)

	if w.Code != http.StatusOK {
		t.Fatalf("export status = %d, want 200", w.Code)
	}
	var exportPayload map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &exportPayload); err != nil {
		t.Fatalf("unmarshal export: %v", err)
	}
	if got := int(exportPayload["version"].(float64)); got != 1 {
		t.Fatalf("export version = %d, want 1", got)
	}
	if _, exists := exportPayload["records"]; exists {
		t.Fatalf("expected no records in legacy memory export, got %v", exportPayload["records"])
	}

	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v0/management/usage/import", strings.NewReader(`{"version":2,"records":[]}`))
	c.Request.Header.Set("Content-Type", "application/json")
	h.ImportUsageStatistics(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("import status = %d, want 400", w.Code)
	}
}

func TestUsageExport_SQLiteReturnsVersionTwoRecords(t *testing.T) {
	gin.SetMode(gin.TestMode)
	original := usage.StatisticsEnabled()
	usage.SetStatisticsEnabled(true)
	t.Cleanup(func() { usage.SetStatisticsEnabled(original) })

	stats := usage.NewRequestStatistics()
	if err := stats.Configure(t.TempDir()); err != nil {
		t.Fatalf("Configure sqlite stats: %v", err)
	}
	stats.Record(context.Background(), coreusage.Record{
		APIKey:      "api-sqlite",
		Model:       "model-sqlite",
		RequestedAt: time.Now().UTC(),
		Detail: coreusage.Detail{
			InputTokens:  6,
			OutputTokens: 7,
			TotalTokens:  13,
		},
	})

	h := &Handler{usageStats: stats}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v0/management/usage/export", nil)
	h.ExportUsageStatistics(c)

	if w.Code != http.StatusOK {
		t.Fatalf("export status = %d, want 200", w.Code)
	}
	var exportPayload map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &exportPayload); err != nil {
		t.Fatalf("unmarshal export: %v", err)
	}
	if got := int(exportPayload["version"].(float64)); got != 2 {
		t.Fatalf("export version = %d, want 2", got)
	}
	records, ok := exportPayload["records"].([]any)
	if !ok || len(records) != 1 {
		t.Fatalf("records = %#v, want len 1", exportPayload["records"])
	}
}
