package usage

import (
	"context"
	"testing"
	"time"

	coreusage "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/usage"
)

func TestRequestStatistics_TokenBreakdownContextSQLite(t *testing.T) {
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
			APIKey:      "api-token",
			Model:       "model-a",
			RequestedAt: time.Date(2026, 3, 9, 12, 10, 0, 0, time.UTC),
			Detail: coreusage.Detail{
				InputTokens:     10,
				OutputTokens:    20,
				CachedTokens:    3,
				ReasoningTokens: 1,
				TotalTokens:     34,
			},
		},
		{
			APIKey:      "api-token",
			Model:       "model-a",
			RequestedAt: time.Date(2026, 3, 9, 10, 20, 0, 0, time.UTC),
			Detail: coreusage.Detail{
				InputTokens:     4,
				OutputTokens:    5,
				CachedTokens:    1,
				ReasoningTokens: 2,
				TotalTokens:     12,
			},
		},
		{
			APIKey:      "api-token",
			Model:       "model-a",
			RequestedAt: time.Date(2026, 3, 9, 6, 5, 0, 0, time.UTC),
			Detail: coreusage.Detail{
				InputTokens:  2,
				OutputTokens: 3,
				TotalTokens:  5,
			},
		},
		{
			APIKey:      "api-token",
			Model:       "model-a",
			RequestedAt: time.Date(2026, 3, 8, 14, 15, 0, 0, time.UTC),
			Detail: coreusage.Detail{
				InputTokens:     6,
				OutputTokens:    7,
				CachedTokens:    2,
				ReasoningTokens: 1,
				TotalTokens:     16,
			},
		},
		{
			APIKey:      "api-token",
			Model:       "model-a",
			RequestedAt: time.Date(2026, 3, 8, 9, 30, 0, 0, time.UTC),
			Detail: coreusage.Detail{
				InputTokens:  8,
				OutputTokens: 1,
				TotalTokens:  9,
			},
		},
		{
			APIKey:      "api-token",
			Model:       "model-b",
			RequestedAt: time.Date(2026, 3, 5, 15, 0, 0, 0, time.UTC),
			Detail: coreusage.Detail{
				InputTokens:     1,
				OutputTokens:    2,
				ReasoningTokens: 4,
				TotalTokens:     7,
			},
		},
		{
			APIKey:      "api-token",
			Model:       "model-c",
			RequestedAt: time.Date(2026, 2, 22, 11, 0, 0, 0, time.UTC),
			Detail: coreusage.Detail{
				InputTokens:     9,
				OutputTokens:    9,
				CachedTokens:    1,
				ReasoningTokens: 1,
				TotalTokens:     20,
			},
		},
		{
			APIKey:      "api-token",
			Model:       "model-d",
			RequestedAt: time.Date(2026, 2, 10, 8, 0, 0, 0, time.UTC),
			Detail: coreusage.Detail{
				InputTokens:  5,
				OutputTokens: 4,
				TotalTokens:  9,
			},
		},
	}
	for _, record := range records {
		stats.Record(context.Background(), record)
	}

	hourly7h, err := stats.TokenBreakdownContext(context.Background(), TokenBreakdownOptions{
		Granularity: tokenBreakdownGranularityHour,
		Range:       tokenBreakdownRange7h,
		Now:         now,
	})
	if err != nil {
		t.Fatalf("hourly 7h breakdown: %v", err)
	}
	if got := len(hourly7h.Buckets); got != 7 {
		t.Fatalf("len(hourly7h.Buckets) = %d, want 7", got)
	}
	assertTokenBreakdownBucket(t, hourly7h, "03-09 14:00", 2, 3, 0, 0)
	assertTokenBreakdownBucket(t, hourly7h, "03-09 18:00", 4, 5, 1, 2)
	assertTokenBreakdownBucket(t, hourly7h, "03-09 20:00", 10, 20, 3, 1)

	hourly24h, err := stats.TokenBreakdownContext(context.Background(), TokenBreakdownOptions{
		Granularity: tokenBreakdownGranularityHour,
		Range:       tokenBreakdownRangeAll,
		Now:         now,
	})
	if err != nil {
		t.Fatalf("hourly all breakdown: %v", err)
	}
	if got := len(hourly24h.Buckets); got != 24 {
		t.Fatalf("len(hourly24h.Buckets) = %d, want 24", got)
	}
	assertTokenBreakdownBucket(t, hourly24h, "03-08 22:00", 6, 7, 2, 1)
	assertNoTokenBreakdownBucket(t, hourly24h, "03-08 09:00")

	dailyCurrent, err := stats.TokenBreakdownContext(context.Background(), TokenBreakdownOptions{
		Granularity: tokenBreakdownGranularityDay,
		Range:       tokenBreakdownRange24h,
		Now:         now,
	})
	if err != nil {
		t.Fatalf("daily current breakdown: %v", err)
	}
	if got := len(dailyCurrent.Buckets); got != 1 {
		t.Fatalf("len(dailyCurrent.Buckets) = %d, want 1", got)
	}
	if dailyCurrent.HasOlder {
		t.Fatalf("dailyCurrent.HasOlder = true, want false")
	}
	assertTokenBreakdownBucket(t, dailyCurrent, "2026-03-09", 16, 28, 4, 3)

	daily7d, err := stats.TokenBreakdownContext(context.Background(), TokenBreakdownOptions{
		Granularity: tokenBreakdownGranularityDay,
		Range:       tokenBreakdownRange7d,
		Now:         now,
	})
	if err != nil {
		t.Fatalf("daily 7d breakdown: %v", err)
	}
	if got := len(daily7d.Buckets); got != 7 {
		t.Fatalf("len(daily7d.Buckets) = %d, want 7", got)
	}
	if daily7d.Buckets[0].Label != "2026-03-03" || daily7d.Buckets[6].Label != "2026-03-09" {
		t.Fatalf("daily7d labels = (%s,%s), want (2026-03-03,2026-03-09)", daily7d.Buckets[0].Label, daily7d.Buckets[6].Label)
	}
	assertTokenBreakdownBucket(t, daily7d, "2026-03-04", 0, 0, 0, 0)
	assertTokenBreakdownBucket(t, daily7d, "2026-03-05", 1, 2, 0, 4)
	assertTokenBreakdownBucket(t, daily7d, "2026-03-08", 14, 8, 2, 1)
	assertTokenBreakdownBucket(t, daily7d, "2026-03-09", 16, 28, 4, 3)

	dailyAll, err := stats.TokenBreakdownContext(context.Background(), TokenBreakdownOptions{
		Granularity: tokenBreakdownGranularityDay,
		Range:       tokenBreakdownRangeAll,
		Now:         now,
	})
	if err != nil {
		t.Fatalf("daily all breakdown: %v", err)
	}
	if got := len(dailyAll.Buckets); got != tokenBreakdownAllPageDays {
		t.Fatalf("len(dailyAll.Buckets) = %d, want %d", got, tokenBreakdownAllPageDays)
	}
	if dailyAll.HasOlder {
		t.Fatalf("dailyAll.HasOlder = true, want false")
	}
	if dailyAll.Buckets[0].Label != "2026-02-08" || dailyAll.Buckets[len(dailyAll.Buckets)-1].Label != "2026-03-09" {
		t.Fatalf("dailyAll labels = (%s,%s), want (2026-02-08,2026-03-09)", dailyAll.Buckets[0].Label, dailyAll.Buckets[len(dailyAll.Buckets)-1].Label)
	}
	assertTokenBreakdownBucket(t, dailyAll, "2026-02-08", 0, 0, 0, 0)
	assertTokenBreakdownBucket(t, dailyAll, "2026-02-10", 5, 4, 0, 0)
	assertTokenBreakdownBucket(t, dailyAll, "2026-02-22", 9, 9, 1, 1)
	assertTokenBreakdownBucket(t, dailyAll, "2026-03-05", 1, 2, 0, 4)
	assertTokenBreakdownBucket(t, dailyAll, "2026-03-09", 16, 28, 4, 3)

	dailyOlderPage, err := stats.TokenBreakdownContext(context.Background(), TokenBreakdownOptions{
		Granularity: tokenBreakdownGranularityDay,
		Range:       tokenBreakdownRangeAll,
		Offset:      tokenBreakdownAllPageDays,
		Now:         now,
	})
	if err != nil {
		t.Fatalf("daily older page breakdown: %v", err)
	}
	if dailyOlderPage.HasOlder {
		t.Fatalf("dailyOlderPage.HasOlder = true, want false")
	}
	if dailyOlderPage.Buckets[0].Label != "2026-01-09" || dailyOlderPage.Buckets[len(dailyOlderPage.Buckets)-1].Label != "2026-02-07" {
		t.Fatalf(
			"dailyOlderPage labels = (%s,%s), want (2026-01-09,2026-02-07)",
			dailyOlderPage.Buckets[0].Label,
			dailyOlderPage.Buckets[len(dailyOlderPage.Buckets)-1].Label,
		)
	}
	assertTokenBreakdownBucket(t, dailyOlderPage, "2026-01-09", 0, 0, 0, 0)
	assertTokenBreakdownBucket(t, dailyOlderPage, "2026-02-07", 0, 0, 0, 0)
}

func TestRequestStatistics_TokenBreakdownContextSQLiteUsesUTCPlus8Buckets(t *testing.T) {
	original := StatisticsEnabled()
	SetStatisticsEnabled(true)
	t.Cleanup(func() { SetStatisticsEnabled(original) })

	stats := NewRequestStatistics()
	if err := stats.Configure(t.TempDir()); err != nil {
		t.Fatalf("configure stats: %v", err)
	}
	t.Cleanup(func() { _ = stats.Close() })

	now := time.Date(2026, 3, 10, 0, 20, 0, 0, time.UTC)
	records := []coreusage.Record{
		{
			APIKey:      "api-token-boundary",
			Model:       "model-a",
			RequestedAt: time.Date(2026, 3, 9, 15, 59, 59, 0, time.UTC),
			Detail: coreusage.Detail{
				InputTokens:  1,
				OutputTokens: 2,
				TotalTokens:  3,
			},
		},
		{
			APIKey:      "api-token-boundary",
			Model:       "model-a",
			RequestedAt: time.Date(2026, 3, 9, 16, 0, 0, 0, time.UTC),
			Detail: coreusage.Detail{
				InputTokens:     3,
				OutputTokens:    4,
				CachedTokens:    1,
				ReasoningTokens: 2,
				TotalTokens:     10,
			},
		},
	}
	for _, record := range records {
		stats.Record(context.Background(), record)
	}

	hourly, err := stats.TokenBreakdownContext(context.Background(), TokenBreakdownOptions{
		Granularity: tokenBreakdownGranularityHour,
		Range:       tokenBreakdownRange24h,
		Now:         now,
	})
	if err != nil {
		t.Fatalf("hourly boundary breakdown: %v", err)
	}
	assertTokenBreakdownBucket(t, hourly, "03-09 23:00", 1, 2, 0, 0)
	assertTokenBreakdownBucket(t, hourly, "03-10 00:00", 3, 4, 1, 2)

	daily, err := stats.TokenBreakdownContext(context.Background(), TokenBreakdownOptions{
		Granularity: tokenBreakdownGranularityDay,
		Range:       tokenBreakdownRange24h,
		Now:         now,
	})
	if err != nil {
		t.Fatalf("daily boundary breakdown: %v", err)
	}
	assertTokenBreakdownBucket(t, daily, "2026-03-10", 3, 4, 1, 2)
}

func assertTokenBreakdownBucket(
	t *testing.T,
	snapshot TokenBreakdownSnapshot,
	label string,
	inputTokens int64,
	outputTokens int64,
	cachedTokens int64,
	reasoningTokens int64,
) {
	t.Helper()
	for _, bucket := range snapshot.Buckets {
		if bucket.Label != label {
			continue
		}
		if bucket.InputTokens != inputTokens ||
			bucket.OutputTokens != outputTokens ||
			bucket.CachedTokens != cachedTokens ||
			bucket.ReasoningTokens != reasoningTokens {
			t.Fatalf(
				"bucket %s = (%d,%d,%d,%d), want (%d,%d,%d,%d)",
				label,
				bucket.InputTokens,
				bucket.OutputTokens,
				bucket.CachedTokens,
				bucket.ReasoningTokens,
				inputTokens,
				outputTokens,
				cachedTokens,
				reasoningTokens,
			)
		}
		return
	}
	t.Fatalf("bucket %s not found", label)
}

func assertNoTokenBreakdownBucket(t *testing.T, snapshot TokenBreakdownSnapshot, label string) {
	t.Helper()
	for _, bucket := range snapshot.Buckets {
		if bucket.Label == label {
			t.Fatalf("bucket %s unexpectedly found", label)
		}
	}
}
