package management

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/usage"
	coreusage "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/usage"
)

func TestGetUsageCostTrend_MemoryReturnsBadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := &Handler{usageStats: usage.NewRequestStatistics()}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v0/management/usage/cost-trend", nil)

	h.GetUsageCostTrend(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestGetUsageCostTrend_SQLiteReturnsSnapshot(t *testing.T) {
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
			InputTokens:  1_000_000,
			OutputTokens: 1_000_000,
			TotalTokens:  2_000_000,
		},
	})

	h := &Handler{
		usageStats: stats,
		cfg: &config.Config{
			ModelPrice: []config.ModelPriceItem{
				{
					Name:      "model-a",
					Input:     mustParseModelPriceValue(t, "1"),
					Output:    mustParseModelPriceValue(t, "2"),
					CacheRead: mustParseModelPriceValue(t, "0.5"),
				},
			},
		},
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(
		http.MethodGet,
		"/v0/management/usage/cost-trend?granularity=day&range=all&offset=0",
		nil,
	)

	h.GetUsageCostTrend(c)

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
	buckets, ok := payload["buckets"].([]any)
	if !ok || len(buckets) != 15 {
		t.Fatalf("buckets = %#v, want len 15", payload["buckets"])
	}
}

func mustParseModelPriceValue(t *testing.T, raw string) config.ModelPriceValue {
	t.Helper()
	var value config.ModelPriceValue
	if err := value.UnmarshalJSON([]byte(`"` + raw + `"`)); err != nil {
		t.Fatalf("parse model price value %s: %v", raw, err)
	}
	return value
}
