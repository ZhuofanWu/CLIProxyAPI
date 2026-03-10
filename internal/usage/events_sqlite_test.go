package usage

import (
	"context"
	"testing"
	"time"

	coreusage "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/usage"
)

func TestRequestStatistics_EventsContextSQLite(t *testing.T) {
	original := StatisticsEnabled()
	SetStatisticsEnabled(true)
	t.Cleanup(func() { SetStatisticsEnabled(original) })

	stats := NewRequestStatistics()
	if err := stats.Configure(t.TempDir()); err != nil {
		t.Fatalf("configure stats: %v", err)
	}
	t.Cleanup(func() { _ = stats.Close() })

	base := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
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
	}
	for _, record := range records {
		stats.Record(context.Background(), record)
	}

	successOnly := true
	successSnapshot, err := stats.EventsContext(context.Background(), UsageEventsOptions{
		Since:     base.Add(-7 * time.Hour),
		Now:       base,
		ModelName: "model-a",
		Source:    "source-a",
		AuthIndex: "auth-1",
		Success:   &successOnly,
		Page:      99,
		PageSize:  1,
	})
	if err != nil {
		t.Fatalf("EventsContext success filter: %v", err)
	}
	if successSnapshot.Total != 2 || successSnapshot.TotalPages != 2 {
		t.Fatalf("success snapshot totals = (%d,%d), want (2,2)", successSnapshot.Total, successSnapshot.TotalPages)
	}
	if successSnapshot.Page != 2 {
		t.Fatalf("success snapshot page = %d, want 2", successSnapshot.Page)
	}
	if !successSnapshot.HasPrev || successSnapshot.HasNext {
		t.Fatalf("success pager flags = (%v,%v), want (true,false)", successSnapshot.HasPrev, successSnapshot.HasNext)
	}
	if len(successSnapshot.Items) != 1 {
		t.Fatalf("len(success items) = %d, want 1", len(successSnapshot.Items))
	}
	if !successSnapshot.Items[0].Timestamp.Equal(base.Add(-10 * time.Minute)) {
		t.Fatalf("success item timestamp = %s, want %s", successSnapshot.Items[0].Timestamp, base.Add(-10*time.Minute))
	}

	failureOnly := false
	failureSnapshot, err := stats.EventsContext(context.Background(), UsageEventsOptions{
		Since:     base.Add(-7 * time.Hour),
		Now:       base,
		ModelName: "model-a",
		Source:    "source-a",
		AuthIndex: "auth-1",
		Success:   &failureOnly,
		Page:      1,
		PageSize:  10,
	})
	if err != nil {
		t.Fatalf("EventsContext failure filter: %v", err)
	}
	if failureSnapshot.Total != 1 || len(failureSnapshot.Items) != 1 {
		t.Fatalf("failure snapshot = total %d len %d, want (1,1)", failureSnapshot.Total, len(failureSnapshot.Items))
	}
	if !failureSnapshot.Items[0].Failed {
		t.Fatalf("failure item failed = false, want true")
	}
}
