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

func TestGetUsageHealth_MemoryReturnsBadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := &Handler{usageStats: usage.NewRequestStatistics()}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v0/management/usage/health", nil)

	h.GetUsageHealth(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestGetUsageHealth_SQLiteReturnsHealthSnapshot(t *testing.T) {
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
		Model:       "model-sqlite",
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
	c.Request = httptest.NewRequest(http.MethodGet, "/v0/management/usage/health", nil)

	h.GetUsageHealth(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var payload struct {
		Rates         []int   `json:"rates"`
		SuccessCounts []int64 `json:"success_counts"`
		FailureCounts []int64 `json:"failure_counts"`
		Rows          int     `json:"rows"`
		Cols          int     `json:"cols"`
		BucketMinutes int     `json:"bucket_minutes"`
		TotalSuccess  int64   `json:"total_success"`
		TotalFailure  int64   `json:"total_failure"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if got := len(payload.Rates); got != 672 {
		t.Fatalf("len(rates) = %d, want 672", got)
	}
	if got := len(payload.SuccessCounts); got != 672 {
		t.Fatalf("len(success_counts) = %d, want 672", got)
	}
	if got := len(payload.FailureCounts); got != 672 {
		t.Fatalf("len(failure_counts) = %d, want 672", got)
	}
	if payload.Rows != 7 || payload.Cols != 96 {
		t.Fatalf("rows/cols = %d/%d, want 7/96", payload.Rows, payload.Cols)
	}
	if payload.BucketMinutes != 15 {
		t.Fatalf("bucket_minutes = %d, want 15", payload.BucketMinutes)
	}
	if payload.TotalSuccess != 1 || payload.TotalFailure != 0 {
		t.Fatalf("totals = (%d,%d), want (1,0)", payload.TotalSuccess, payload.TotalFailure)
	}
	if payload.Rates[671] != 100 {
		t.Fatalf("rates[671] = %d, want 100", payload.Rates[671])
	}
}
