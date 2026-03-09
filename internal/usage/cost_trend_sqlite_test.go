package usage

import (
	"context"
	"math"
	"testing"
	"time"

	coreusage "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/usage"
)

func TestRequestStatistics_CostTrendContextSQLite(t *testing.T) {
	original := StatisticsEnabled()
	SetStatisticsEnabled(true)
	t.Cleanup(func() { SetStatisticsEnabled(original) })

	stats := NewRequestStatistics()
	if err := stats.Configure(t.TempDir()); err != nil {
		t.Fatalf("configure stats: %v", err)
	}
	t.Cleanup(func() { _ = stats.Close() })

	now := time.Date(2026, 3, 9, 12, 34, 0, 0, time.UTC)
	records := []coreusage.Record{
		{
			APIKey:      "api-cost",
			Model:       "model-a",
			RequestedAt: time.Date(2026, 3, 9, 12, 10, 0, 0, time.UTC),
			Detail: coreusage.Detail{
				InputTokens:  4_000_000,
				OutputTokens: 2_000_000,
				CachedTokens: 1_000_000,
				TotalTokens:  7_000_000,
			},
		},
		{
			APIKey:      "api-cost",
			Model:       "model-b",
			RequestedAt: time.Date(2026, 3, 9, 10, 20, 0, 0, time.UTC),
			Detail: coreusage.Detail{
				InputTokens:  1_000_000,
				OutputTokens: 3_000_000,
				TotalTokens:  4_000_000,
			},
		},
		{
			APIKey:      "api-cost",
			Model:       "model-a",
			RequestedAt: time.Date(2026, 3, 9, 6, 5, 0, 0, time.UTC),
			Detail: coreusage.Detail{
				InputTokens:  2_000_000,
				OutputTokens: 1_000_000,
				TotalTokens:  3_000_000,
			},
		},
		{
			APIKey:      "api-cost",
			Model:       "model-a",
			RequestedAt: time.Date(2026, 3, 8, 14, 15, 0, 0, time.UTC),
			Detail: coreusage.Detail{
				InputTokens:  1_000_000,
				OutputTokens: 1_000_000,
				TotalTokens:  2_000_000,
			},
		},
		{
			APIKey:      "api-cost",
			Model:       "model-b",
			RequestedAt: time.Date(2026, 3, 5, 15, 0, 0, 0, time.UTC),
			Detail: coreusage.Detail{
				InputTokens:  2_000_000,
				OutputTokens: 1_000_000,
				CachedTokens: 1_000_000,
				TotalTokens:  4_000_000,
			},
		},
		{
			APIKey:      "api-cost",
			Model:       "model-a",
			RequestedAt: time.Date(2026, 2, 22, 11, 0, 0, 0, time.UTC),
			Detail: coreusage.Detail{
				InputTokens:  1_000_000,
				OutputTokens: 1_000_000,
				TotalTokens:  2_000_000,
			},
		},
		{
			APIKey:      "api-cost",
			Model:       "model-b",
			RequestedAt: time.Date(2026, 2, 10, 8, 0, 0, 0, time.UTC),
			Detail: coreusage.Detail{
				InputTokens:  2_000_000,
				OutputTokens: 2_000_000,
				TotalTokens:  4_000_000,
			},
		},
	}
	for _, record := range records {
		stats.Record(context.Background(), record)
	}

	modelPrices := map[string]ModelPrice{
		"model-a": {PromptMilli: 1000, CompletionMilli: 2000, CacheMilli: 500},
		"model-b": {PromptMilli: 2000, CompletionMilli: 1000, CacheMilli: 1000},
	}

	hourly7h, err := stats.CostTrendContext(context.Background(), CostTrendOptions{
		Granularity: tokenBreakdownGranularityHour,
		Range:       tokenBreakdownRange7h,
		Now:         now,
		ModelPrices: modelPrices,
	})
	if err != nil {
		t.Fatalf("hourly 7h cost trend: %v", err)
	}
	if got := len(hourly7h.Buckets); got != 7 {
		t.Fatalf("len(hourly7h.Buckets) = %d, want 7", got)
	}
	assertCostTrendBucket(t, hourly7h, "03-09 06:00", 4)
	assertCostTrendBucket(t, hourly7h, "03-09 10:00", 5)
	assertCostTrendBucket(t, hourly7h, "03-09 12:00", 7.5)

	hourly24h, err := stats.CostTrendContext(context.Background(), CostTrendOptions{
		Granularity: tokenBreakdownGranularityHour,
		Range:       tokenBreakdownRangeAll,
		Now:         now,
		ModelPrices: modelPrices,
	})
	if err != nil {
		t.Fatalf("hourly all cost trend: %v", err)
	}
	if got := len(hourly24h.Buckets); got != 24 {
		t.Fatalf("len(hourly24h.Buckets) = %d, want 24", got)
	}
	assertCostTrendBucket(t, hourly24h, "03-08 14:00", 3)
	assertCostTrendBucket(t, hourly24h, "03-09 06:00", 4)
	assertCostTrendBucket(t, hourly24h, "03-09 10:00", 5)
	assertCostTrendBucket(t, hourly24h, "03-09 12:00", 7.5)

	dailyCurrent, err := stats.CostTrendContext(context.Background(), CostTrendOptions{
		Granularity: tokenBreakdownGranularityDay,
		Range:       tokenBreakdownRange24h,
		Now:         now,
		ModelPrices: modelPrices,
	})
	if err != nil {
		t.Fatalf("daily current cost trend: %v", err)
	}
	if got := len(dailyCurrent.Buckets); got != 1 {
		t.Fatalf("len(dailyCurrent.Buckets) = %d, want 1", got)
	}
	assertCostTrendBucket(t, dailyCurrent, "2026-03-09", 16.5)

	daily7d, err := stats.CostTrendContext(context.Background(), CostTrendOptions{
		Granularity: tokenBreakdownGranularityDay,
		Range:       tokenBreakdownRange7d,
		Now:         now,
		ModelPrices: modelPrices,
	})
	if err != nil {
		t.Fatalf("daily 7d cost trend: %v", err)
	}
	assertCostTrendBucket(t, daily7d, "2026-03-05", 4)
	assertCostTrendBucket(t, daily7d, "2026-03-08", 3)
	assertCostTrendBucket(t, daily7d, "2026-03-09", 16.5)

	dailyAll, err := stats.CostTrendContext(context.Background(), CostTrendOptions{
		Granularity: tokenBreakdownGranularityDay,
		Range:       tokenBreakdownRangeAll,
		Now:         now,
		ModelPrices: modelPrices,
	})
	if err != nil {
		t.Fatalf("daily all cost trend: %v", err)
	}
	if !dailyAll.HasOlder {
		t.Fatalf("dailyAll.HasOlder = false, want true")
	}
	assertCostTrendBucket(t, dailyAll, "2026-03-05", 4)
	assertCostTrendBucket(t, dailyAll, "2026-03-09", 16.5)

	dailyOlderPage, err := stats.CostTrendContext(context.Background(), CostTrendOptions{
		Granularity: tokenBreakdownGranularityDay,
		Range:       tokenBreakdownRangeAll,
		Offset:      tokenBreakdownAllPageDays,
		Now:         now,
		ModelPrices: modelPrices,
	})
	if err != nil {
		t.Fatalf("daily older page cost trend: %v", err)
	}
	if dailyOlderPage.HasOlder {
		t.Fatalf("dailyOlderPage.HasOlder = true, want false")
	}
	assertCostTrendBucket(t, dailyOlderPage, "2026-02-10", 6)
	assertCostTrendBucket(t, dailyOlderPage, "2026-02-22", 3)
}

func assertCostTrendBucket(t *testing.T, snapshot CostTrendSnapshot, label string, cost float64) {
	t.Helper()
	for _, bucket := range snapshot.Buckets {
		if bucket.Label != label {
			continue
		}
		if diff := math.Abs(bucket.Cost - cost); diff > 1e-9 {
			t.Fatalf("bucket %s cost = %.9f, want %.9f", label, bucket.Cost, cost)
		}
		return
	}
	t.Fatalf("bucket %s not found", label)
}
