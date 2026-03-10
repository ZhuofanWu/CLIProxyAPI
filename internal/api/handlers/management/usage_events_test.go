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

func TestGetUsageEvents_MemoryReturnsBadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := &Handler{usageStats: usage.NewRequestStatistics()}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v0/management/usage/events?page=1&page_size=100", nil)

	h.GetUsageEvents(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestGetUsageEvents_SQLiteSupportsFiltersAndPagination(t *testing.T) {
	gin.SetMode(gin.TestMode)
	original := usage.StatisticsEnabled()
	usage.SetStatisticsEnabled(true)
	t.Cleanup(func() { usage.SetStatisticsEnabled(original) })

	stats := usage.NewRequestStatistics()
	if err := stats.Configure(t.TempDir()); err != nil {
		t.Fatalf("Configure sqlite stats: %v", err)
	}
	t.Cleanup(func() { _ = stats.Close() })

	base := time.Now().UTC().Truncate(time.Second)
	records := []coreusage.Record{
		{
			APIKey:      "api-1",
			Model:       "model-a",
			Source:      "source-a",
			AuthIndex:   "auth-1",
			RequestedAt: base.Add(-10 * time.Minute),
			Detail:      coreusage.Detail{InputTokens: 3, OutputTokens: 2, TotalTokens: 5},
		},
		{
			APIKey:      "api-1",
			Model:       "model-a",
			Source:      "source-a",
			AuthIndex:   "auth-1",
			RequestedAt: base.Add(-5 * time.Minute),
			Detail:      coreusage.Detail{InputTokens: 4, OutputTokens: 3, TotalTokens: 7},
		},
		{
			APIKey:      "api-1",
			Model:       "model-a",
			Source:      "source-a",
			AuthIndex:   "auth-1",
			RequestedAt: base.Add(-1 * time.Minute),
			Failed:      true,
			Detail:      coreusage.Detail{InputTokens: 1, OutputTokens: 0, TotalTokens: 1},
		},
		{
			APIKey:      "api-2",
			Model:       "model-b",
			Source:      "source-b",
			AuthIndex:   "auth-2",
			RequestedAt: base.Add(-2 * time.Minute),
			Detail:      coreusage.Detail{InputTokens: 2, OutputTokens: 2, TotalTokens: 4},
		},
		{
			APIKey:      "api-1",
			Model:       "model-a",
			Source:      "source-a",
			AuthIndex:   "auth-9",
			RequestedAt: base.Add(-8 * time.Hour),
			Detail:      coreusage.Detail{InputTokens: 9, OutputTokens: 1, TotalTokens: 10},
		},
	}
	for _, record := range records {
		stats.Record(context.Background(), record)
	}

	h := &Handler{usageStats: stats}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(
		http.MethodGet,
		"/v0/management/usage/events?range=all&model=model-a&source=source-a&auth_index=auth-1&success=true&page=2&page_size=1",
		nil,
	)

	h.GetUsageEvents(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var payload struct {
		Page       int   `json:"page"`
		PageSize   int   `json:"page_size"`
		Total      int64 `json:"total"`
		TotalPages int   `json:"total_pages"`
		HasPrev    bool  `json:"has_prev"`
		HasNext    bool  `json:"has_next"`
		Items      []struct {
			ModelName string `json:"model_name"`
			Source    string `json:"source"`
			AuthIndex string `json:"auth_index"`
			Failed    bool   `json:"failed"`
			Timestamp string `json:"timestamp"`
		} `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if payload.Page != 2 || payload.PageSize != 1 {
		t.Fatalf("page info = (%d,%d), want (2,1)", payload.Page, payload.PageSize)
	}
	if payload.Total != 2 || payload.TotalPages != 2 {
		t.Fatalf("total info = (%d,%d), want (2,2)", payload.Total, payload.TotalPages)
	}
	if !payload.HasPrev || payload.HasNext {
		t.Fatalf("pager flags = (%v,%v), want (true,false)", payload.HasPrev, payload.HasNext)
	}
	if len(payload.Items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(payload.Items))
	}
	item := payload.Items[0]
	if item.ModelName != "model-a" || item.Source != "source-a" || item.AuthIndex != "auth-1" {
		t.Fatalf("item identity = (%q,%q,%q)", item.ModelName, item.Source, item.AuthIndex)
	}
	if item.Failed {
		t.Fatalf("item failed = true, want false")
	}
	if item.Timestamp != base.Add(-10*time.Minute).Format(time.RFC3339Nano) {
		t.Fatalf("timestamp = %q, want %q", item.Timestamp, base.Add(-10*time.Minute).Format(time.RFC3339Nano))
	}
}
