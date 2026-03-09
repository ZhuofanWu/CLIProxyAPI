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

func TestGetUsageTokenBreakdown_MemoryReturnsBadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := &Handler{usageStats: usage.NewRequestStatistics()}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v0/management/usage/token-breakdown", nil)

	h.GetUsageTokenBreakdown(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestGetUsageTokenBreakdown_InvalidOffsetReturnsBadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := &Handler{usageStats: usage.NewRequestStatistics()}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(
		http.MethodGet,
		"/v0/management/usage/token-breakdown?granularity=day&range=all&offset=bad",
		nil,
	)

	h.GetUsageTokenBreakdown(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestGetUsageTokenBreakdown_SQLiteReturnsSnapshot(t *testing.T) {
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
		Detail: coreusage.Detail{
			InputTokens:     6,
			OutputTokens:    7,
			CachedTokens:    2,
			ReasoningTokens: 1,
			TotalTokens:     16,
		},
	})

	h := &Handler{usageStats: stats}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(
		http.MethodGet,
		"/v0/management/usage/token-breakdown?granularity=day&range=all&offset=0",
		nil,
	)

	h.GetUsageTokenBreakdown(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var payload map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if payload["granularity"] != "day" {
		t.Fatalf("granularity = %v, want day", payload["granularity"])
	}
	if payload["range"] != "all" {
		t.Fatalf("range = %v, want all", payload["range"])
	}
	if got := int(payload["offset"].(float64)); got != 0 {
		t.Fatalf("offset = %d, want 0", got)
	}
	buckets, ok := payload["buckets"].([]any)
	if !ok || len(buckets) != 30 {
		t.Fatalf("buckets = %#v, want len 30", payload["buckets"])
	}
}
