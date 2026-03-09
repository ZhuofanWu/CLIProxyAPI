package usage

import (
	"context"
	"math"
	"testing"
	"time"

	coreusage "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/usage"
)

func TestRequestStatistics_HealthContextSQLite(t *testing.T) {
	original := StatisticsEnabled()
	SetStatisticsEnabled(true)
	t.Cleanup(func() { SetStatisticsEnabled(original) })

	stats := NewRequestStatistics()
	if err := stats.Configure(t.TempDir()); err != nil {
		t.Fatalf("configure stats: %v", err)
	}
	t.Cleanup(func() { _ = stats.Close() })

	now := time.Date(2026, 3, 9, 12, 0, 0, 0, time.UTC)
	windowStart := now.Add(-usageHealthWindowDuration)
	records := []coreusage.Record{
		{APIKey: "api-health", Model: "model-a", RequestedAt: now, Detail: coreusage.Detail{TotalTokens: 1}},
		{APIKey: "api-health", Model: "model-a", RequestedAt: now.Add(-10 * time.Minute), Detail: coreusage.Detail{TotalTokens: 1}},
		{APIKey: "api-health", Model: "model-a", RequestedAt: now.Add(-5 * time.Minute), Failed: true, Detail: coreusage.Detail{TotalTokens: 1}},
		{APIKey: "api-health", Model: "model-a", RequestedAt: now.Add(-20 * time.Minute), Detail: coreusage.Detail{TotalTokens: 1}},
		{APIKey: "api-health", Model: "model-a", RequestedAt: windowStart, Detail: coreusage.Detail{TotalTokens: 1}},
		{APIKey: "api-health", Model: "model-a", RequestedAt: windowStart.Add(time.Second), Failed: true, Detail: coreusage.Detail{TotalTokens: 1}},
	}
	for _, record := range records {
		stats.Record(context.Background(), record)
	}

	health, err := stats.HealthContext(context.Background(), HealthOptions{Now: now})
	if err != nil {
		t.Fatalf("HealthContext: %v", err)
	}

	if got := len(health.Rates); got != usageHealthBucketCount {
		t.Fatalf("len(Rates) = %d, want %d", got, usageHealthBucketCount)
	}
	if got := len(health.SuccessCounts); got != usageHealthBucketCount {
		t.Fatalf("len(SuccessCounts) = %d, want %d", got, usageHealthBucketCount)
	}
	if got := len(health.FailureCounts); got != usageHealthBucketCount {
		t.Fatalf("len(FailureCounts) = %d, want %d", got, usageHealthBucketCount)
	}
	if health.Rows != usageHealthRows {
		t.Fatalf("Rows = %d, want %d", health.Rows, usageHealthRows)
	}
	if health.Cols != usageHealthCols {
		t.Fatalf("Cols = %d, want %d", health.Cols, usageHealthCols)
	}
	if health.BucketMinutes != usageHealthBucketMinutes {
		t.Fatalf("BucketMinutes = %d, want %d", health.BucketMinutes, usageHealthBucketMinutes)
	}
	if !health.WindowStart.Equal(windowStart) {
		t.Fatalf("WindowStart = %s, want %s", health.WindowStart, windowStart)
	}
	if !health.WindowEnd.Equal(now) {
		t.Fatalf("WindowEnd = %s, want %s", health.WindowEnd, now)
	}

	if health.Rates[0] != 0 {
		t.Fatalf("Rates[0] = %d, want 0", health.Rates[0])
	}
	if health.SuccessCounts[0] != 0 || health.FailureCounts[0] != 1 {
		t.Fatalf("bucket 0 counts = (%d,%d), want (0,1)", health.SuccessCounts[0], health.FailureCounts[0])
	}
	if health.Rates[1] != -1 {
		t.Fatalf("Rates[1] = %d, want -1", health.Rates[1])
	}
	if health.Rates[670] != 100 {
		t.Fatalf("Rates[670] = %d, want 100", health.Rates[670])
	}
	if health.SuccessCounts[670] != 1 || health.FailureCounts[670] != 0 {
		t.Fatalf("bucket 670 counts = (%d,%d), want (1,0)", health.SuccessCounts[670], health.FailureCounts[670])
	}
	if health.Rates[671] != 67 {
		t.Fatalf("Rates[671] = %d, want 67", health.Rates[671])
	}
	if health.SuccessCounts[671] != 2 || health.FailureCounts[671] != 1 {
		t.Fatalf("bucket 671 counts = (%d,%d), want (2,1)", health.SuccessCounts[671], health.FailureCounts[671])
	}
	if health.TotalSuccess != 3 {
		t.Fatalf("TotalSuccess = %d, want 3", health.TotalSuccess)
	}
	if health.TotalFailure != 2 {
		t.Fatalf("TotalFailure = %d, want 2", health.TotalFailure)
	}
	if diff := math.Abs(health.SuccessRate - 60); diff > 1e-9 {
		t.Fatalf("SuccessRate = %.9f, want 60", health.SuccessRate)
	}
}
