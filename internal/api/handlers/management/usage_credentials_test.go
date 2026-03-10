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

func TestGetUsageCredentials_MemoryReturnsBadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := &Handler{usageStats: usage.NewRequestStatistics()}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v0/management/usage/credentials?range=all", nil)

	h.GetUsageCredentials(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestGetUsageCredentials_SQLiteReturnsSnapshot(t *testing.T) {
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
		Source:      "source-a",
		AuthIndex:   "auth-1",
		RequestedAt: now,
		Detail:      coreusage.Detail{TotalTokens: 1},
	})
	stats.Record(context.Background(), coreusage.Record{
		APIKey:      "api-sqlite",
		Model:       "model-a",
		Source:      "source-a",
		AuthIndex:   "auth-1",
		RequestedAt: now.Add(-5 * time.Minute),
		Failed:      true,
		Detail:      coreusage.Detail{TotalTokens: 1},
	})

	h := &Handler{usageStats: stats}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(
		http.MethodGet,
		"/v0/management/usage/credentials?range=all&percentdata=true",
		nil,
	)

	h.GetUsageCredentials(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var payload struct {
		Range       string `json:"range"`
		PercentData bool   `json:"percentdata"`
		Credentials []struct {
			Source      string  `json:"source"`
			AuthIndex   string  `json:"auth_index"`
			Success     int64   `json:"success"`
			Failure     int64   `json:"failure"`
			Total       int64   `json:"total"`
			SuccessRate float64 `json:"success_rate"`
			Health      struct {
				Rates         []int   `json:"rates"`
				SuccessCounts []int64 `json:"success_counts"`
				FailureCounts []int64 `json:"failure_counts"`
				Rows          int     `json:"rows"`
				Cols          int     `json:"cols"`
				BucketMinutes int     `json:"bucket_minutes"`
				TotalSuccess  int64   `json:"total_success"`
				TotalFailure  int64   `json:"total_failure"`
			} `json:"health"`
		} `json:"credentials"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if payload.Range != "all" {
		t.Fatalf("range = %q, want all", payload.Range)
	}
	if !payload.PercentData {
		t.Fatalf("percentdata = false, want true")
	}
	if len(payload.Credentials) != 1 {
		t.Fatalf("len(credentials) = %d, want 1", len(payload.Credentials))
	}

	item := payload.Credentials[0]
	if item.Source != "source-a" || item.AuthIndex != "auth-1" {
		t.Fatalf("credential identity = (%q,%q), want (source-a,auth-1)", item.Source, item.AuthIndex)
	}
	if item.Success != 1 || item.Failure != 1 || item.Total != 2 {
		t.Fatalf("counts = (%d,%d,%d), want (1,1,2)", item.Success, item.Failure, item.Total)
	}
	if item.Health.Rows != 1 || item.Health.Cols != 20 {
		t.Fatalf("rows/cols = %d/%d, want 1/20", item.Health.Rows, item.Health.Cols)
	}
	if item.Health.BucketMinutes != 15 {
		t.Fatalf("bucket_minutes = %d, want 15", item.Health.BucketMinutes)
	}
	if got := len(item.Health.Rates); got != 20 {
		t.Fatalf("len(rates) = %d, want 20", got)
	}
	if got := len(item.Health.SuccessCounts); got != 20 {
		t.Fatalf("len(success_counts) = %d, want 20", got)
	}
	if got := len(item.Health.FailureCounts); got != 20 {
		t.Fatalf("len(failure_counts) = %d, want 20", got)
	}
	if item.Health.TotalSuccess != 1 || item.Health.TotalFailure != 1 {
		t.Fatalf("health totals = (%d,%d), want (1,1)", item.Health.TotalSuccess, item.Health.TotalFailure)
	}
	if item.Health.Rates[19] != 50 {
		t.Fatalf("rates[19] = %d, want 50", item.Health.Rates[19])
	}
}
