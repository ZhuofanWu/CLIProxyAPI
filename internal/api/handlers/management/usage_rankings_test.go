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

func TestGetUsageRankings_MemoryReturnsBadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := &Handler{usageStats: usage.NewRequestStatistics()}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v0/management/usage/rankings?range=24h", nil)

	h.GetUsageRankings(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestGetUsageRankings_SQLiteReturnsSnapshot(t *testing.T) {
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
		"/v0/management/usage/rankings?range=all",
		nil,
	)

	h.GetUsageRankings(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var payload map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	apiRankings, ok := payload["api_rankings"].([]any)
	if !ok || len(apiRankings) != 1 {
		t.Fatalf("api_rankings = %#v, want len 1", payload["api_rankings"])
	}
	modelRankings, ok := payload["model_rankings"].([]any)
	if !ok || len(modelRankings) != 1 {
		t.Fatalf("model_rankings = %#v, want len 1", payload["model_rankings"])
	}
	firstAPI, ok := apiRankings[0].(map[string]any)
	if !ok {
		t.Fatalf("first api ranking = %#v, want object", apiRankings[0])
	}
	if firstAPI["api_name"] != "api-sqlite" {
		t.Fatalf("api_name = %v, want api-sqlite", firstAPI["api_name"])
	}
	if firstAPI["total_requests"] != float64(1) {
		t.Fatalf("total_requests = %v, want 1", firstAPI["total_requests"])
	}
	if firstAPI["total_cost"] != 3.0 {
		t.Fatalf("total_cost = %v, want 3", firstAPI["total_cost"])
	}
}
