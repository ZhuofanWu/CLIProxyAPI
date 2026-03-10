package management

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/usage"
	coreusage "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/usage"
)

func TestGetUsageRequestTrend_MemoryReturnsBadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := &Handler{usageStats: usage.NewRequestStatistics()}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v0/management/usage/request-trend", nil)

	h.GetUsageRequestTrend(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestGetUsageRequestTrend_InvalidOffsetReturnsBadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := &Handler{usageStats: usage.NewRequestStatistics()}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(
		http.MethodGet,
		"/v0/management/usage/request-trend?granularity=day&range=all&offset=bad",
		nil,
	)

	h.GetUsageRequestTrend(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestGetUsageTokenTrend_SQLiteReturnsMetricValues(t *testing.T) {
	gin.SetMode(gin.TestMode)
	original := usage.StatisticsEnabled()
	usage.SetStatisticsEnabled(true)
	t.Cleanup(func() { usage.SetStatisticsEnabled(original) })

	stats := usage.NewRequestStatistics()
	if err := stats.Configure(t.TempDir()); err != nil {
		t.Fatalf("Configure sqlite stats: %v", err)
	}
	t.Cleanup(func() { _ = stats.Close() })

	now := time.Now().UTC()
	stats.Record(context.Background(), coreusage.Record{
		APIKey:      "api-sqlite",
		Model:       "model-a",
		RequestedAt: now,
		Detail:      coreusage.Detail{TotalTokens: 13},
	})

	h := &Handler{usageStats: stats}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(
		http.MethodGet,
		"/v0/management/usage/token-trend?granularity=hour&range=24h&offset=15&model=all&model=model-a",
		nil,
	)

	h.GetUsageTokenTrend(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var payload usage.MetricTrendSnapshot
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if payload.Metric != "tokens" {
		t.Fatalf("metric = %q, want tokens", payload.Metric)
	}
	if payload.Granularity != "hour" {
		t.Fatalf("granularity = %q, want hour", payload.Granularity)
	}
	if payload.Range != "24h" {
		t.Fatalf("range = %q, want 24h", payload.Range)
	}
	if payload.Offset != 0 || payload.HasOlder {
		t.Fatalf("pagination = (%d,%t), want (0,false)", payload.Offset, payload.HasOlder)
	}
	if len(payload.Series) != 2 {
		t.Fatalf("len(payload.Series) = %d, want 2", len(payload.Series))
	}
	if payload.Series[0].ModelName != "all" || !payload.Series[0].IsAll {
		t.Fatalf("payload.Series[0] = %#v, want all series", payload.Series[0])
	}
}

func TestGetUsageRequestTrend_SQLiteDayAllReturnsPagination(t *testing.T) {
	gin.SetMode(gin.TestMode)
	original := usage.StatisticsEnabled()
	usage.SetStatisticsEnabled(true)
	t.Cleanup(func() { usage.SetStatisticsEnabled(original) })

	stats := usage.NewRequestStatistics()
	if err := stats.Configure(t.TempDir()); err != nil {
		t.Fatalf("Configure sqlite stats: %v", err)
	}
	t.Cleanup(func() { _ = stats.Close() })

	for _, ts := range []time.Time{
		time.Date(2026, 3, 10, 11, 0, 0, 0, time.UTC),
		time.Date(2026, 2, 20, 9, 0, 0, 0, time.UTC),
	} {
		stats.Record(context.Background(), coreusage.Record{
			APIKey:      "api-sqlite",
			Model:       "model-a",
			RequestedAt: ts,
			Detail:      coreusage.Detail{TotalTokens: 9},
		})
	}

	h := &Handler{usageStats: stats}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(
		http.MethodGet,
		"/v0/management/usage/request-trend?granularity=day&range=all&model=all",
		nil,
	)

	h.GetUsageRequestTrend(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var payload usage.MetricTrendSnapshot
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if payload.Offset != 0 || !payload.HasOlder {
		t.Fatalf("pagination = (%d,%t), want (0,true)", payload.Offset, payload.HasOlder)
	}
	if got := len(payload.Labels); got != 15 {
		t.Fatalf("len(payload.Labels) = %d, want 15", got)
	}
}

func TestGetUsageModels_SQLiteReturnsSnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	original := usage.StatisticsEnabled()
	usage.SetStatisticsEnabled(true)
	t.Cleanup(func() { usage.SetStatisticsEnabled(original) })

	stats := usage.NewRequestStatistics()
	if err := stats.Configure(t.TempDir()); err != nil {
		t.Fatalf("Configure sqlite stats: %v", err)
	}
	t.Cleanup(func() { _ = stats.Close() })

	now := time.Now().UTC()
	stats.Record(context.Background(), coreusage.Record{
		APIKey:      "api-sqlite",
		Model:       "model-a",
		RequestedAt: now,
		Detail:      coreusage.Detail{TotalTokens: 9},
	})

	h := &Handler{usageStats: stats}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v0/management/usage/models?range=24h", nil)

	h.GetUsageModels(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var payload usage.TrendModelsSnapshot
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if payload.Range != "24h" {
		t.Fatalf("range = %q, want 24h", payload.Range)
	}
	if len(payload.Models) != 1 || payload.Models[0].ModelName != "model-a" {
		t.Fatalf("payload.Models = %#v, want model-a", payload.Models)
	}
}
