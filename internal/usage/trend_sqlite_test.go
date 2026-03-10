package usage

import (
	"context"
	"testing"
	"time"

	coreusage "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/usage"
)

func TestRequestStatistics_TrendContextSQLite(t *testing.T) {
	original := StatisticsEnabled()
	SetStatisticsEnabled(true)
	t.Cleanup(func() { SetStatisticsEnabled(original) })

	stats := NewRequestStatistics()
	if err := stats.Configure(t.TempDir()); err != nil {
		t.Fatalf("configure stats: %v", err)
	}
	t.Cleanup(func() { _ = stats.Close() })

	now := time.Date(2026, 3, 10, 12, 34, 0, 0, time.UTC)
	records := []coreusage.Record{
		{
			APIKey:      "api-trend",
			Model:       "model-a",
			RequestedAt: time.Date(2026, 3, 10, 12, 5, 0, 0, time.UTC),
			Detail:      coreusage.Detail{TotalTokens: 15},
		},
		{
			APIKey:      "api-trend",
			Model:       "model-a",
			RequestedAt: time.Date(2026, 3, 10, 10, 10, 0, 0, time.UTC),
			Detail:      coreusage.Detail{TotalTokens: 7},
		},
		{
			APIKey:      "api-trend",
			Model:       "model-b",
			RequestedAt: time.Date(2026, 3, 10, 12, 15, 0, 0, time.UTC),
			Detail:      coreusage.Detail{TotalTokens: 20},
		},
		{
			APIKey:      "api-trend",
			Model:       "model-c",
			RequestedAt: time.Date(2026, 3, 10, 8, 0, 0, 0, time.UTC),
			Detail:      coreusage.Detail{TotalTokens: 30},
		},
		{
			APIKey:      "api-trend",
			Model:       "model-d",
			RequestedAt: time.Date(2026, 3, 9, 23, 0, 0, 0, time.UTC),
			Detail:      coreusage.Detail{TotalTokens: 11},
		},
		{
			APIKey:      "api-trend",
			Model:       "model-e",
			RequestedAt: time.Date(2026, 3, 5, 9, 0, 0, 0, time.UTC),
			Detail:      coreusage.Detail{TotalTokens: 40},
		},
		{
			APIKey:      "api-trend",
			Model:       "model-old",
			RequestedAt: time.Date(2026, 2, 20, 9, 0, 0, 0, time.UTC),
			Detail:      coreusage.Detail{TotalTokens: 99},
		},
	}
	for _, record := range records {
		stats.Record(context.Background(), record)
	}

	hourly7h, err := stats.TrendContext(context.Background(), TrendOptions{
		Granularity: trendGranularityHour,
		Range:       trendRange7h,
		Now:         now,
		Models:      []string{trendAllModelName, "model-a", "model-c"},
	})
	if err != nil {
		t.Fatalf("hourly 7h trend: %v", err)
	}
	if got := len(hourly7h.Labels); got != 7 {
		t.Fatalf("len(hourly7h.Labels) = %d, want 7", got)
	}
	if hourly7h.Offset != 0 || hourly7h.HasOlder {
		t.Fatalf("hourly7h pagination = (%d,%t), want (0,false)", hourly7h.Offset, hourly7h.HasOlder)
	}
	assertTrendSeriesValue(t, hourly7h, trendAllModelName, "03-10 16:00", 1, 30)
	assertTrendSeriesValue(t, hourly7h, trendAllModelName, "03-10 18:00", 1, 7)
	assertTrendSeriesValue(t, hourly7h, trendAllModelName, "03-10 20:00", 2, 35)

	hourlyAll, err := stats.TrendContext(context.Background(), TrendOptions{
		Granularity: trendGranularityHour,
		Range:       trendRangeAll,
		Offset:      trendAllPageDays,
		Now:         now,
		Models:      []string{trendAllModelName, "model-d"},
	})
	if err != nil {
		t.Fatalf("hourly all trend: %v", err)
	}
	if got := len(hourlyAll.Labels); got != 24 {
		t.Fatalf("len(hourlyAll.Labels) = %d, want 24", got)
	}
	if hourlyAll.Offset != 0 || hourlyAll.HasOlder {
		t.Fatalf("hourlyAll pagination = (%d,%t), want (0,false)", hourlyAll.Offset, hourlyAll.HasOlder)
	}
	assertTrendSeriesValue(t, hourlyAll, trendAllModelName, "03-10 07:00", 1, 11)
	assertTrendSeriesValue(t, hourlyAll, trendAllModelName, "03-10 20:00", 2, 35)
	assertTrendSeriesValue(t, hourlyAll, "model-d", "03-10 07:00", 1, 11)

	daily7h, err := stats.TrendContext(context.Background(), TrendOptions{
		Granularity: trendGranularityDay,
		Range:       trendRange7h,
		Now:         now,
		Models:      []string{trendAllModelName, "model-d"},
	})
	if err != nil {
		t.Fatalf("daily 7h trend: %v", err)
	}
	if got := len(daily7h.Labels); got != 1 {
		t.Fatalf("len(daily7h.Labels) = %d, want 1", got)
	}
	if daily7h.Labels[0] != "2026-03-10" {
		t.Fatalf("daily7h label = %q, want 2026-03-10", daily7h.Labels[0])
	}
	assertTrendSeriesValue(t, daily7h, trendAllModelName, "2026-03-10", 5, 83)
	assertTrendSeriesValue(t, daily7h, "model-d", "2026-03-10", 1, 11)

	daily7d, err := stats.TrendContext(context.Background(), TrendOptions{
		Granularity: trendGranularityDay,
		Range:       trendRange7d,
		Now:         now,
		Models:      []string{trendAllModelName, "model-e"},
	})
	if err != nil {
		t.Fatalf("daily 7d trend: %v", err)
	}
	if got := len(daily7d.Labels); got != 7 {
		t.Fatalf("len(daily7d.Labels) = %d, want 7", got)
	}
	if daily7d.Labels[0] != "2026-03-04" || daily7d.Labels[6] != "2026-03-10" {
		t.Fatalf("daily7d labels = (%s,%s), want (2026-03-04,2026-03-10)", daily7d.Labels[0], daily7d.Labels[6])
	}
	assertTrendSeriesValue(t, daily7d, trendAllModelName, "2026-03-05", 1, 40)
	assertTrendSeriesValue(t, daily7d, trendAllModelName, "2026-03-10", 5, 83)
	assertTrendSeriesValue(t, daily7d, "model-e", "2026-03-05", 1, 40)

	dailyAll, err := stats.TrendContext(context.Background(), TrendOptions{
		Granularity: trendGranularityDay,
		Range:       trendRangeAll,
		Now:         now,
		Models:      []string{trendAllModelName, "model-old"},
	})
	if err != nil {
		t.Fatalf("daily all trend: %v", err)
	}
	if got := len(dailyAll.Labels); got != trendAllPageDays {
		t.Fatalf("len(dailyAll.Labels) = %d, want %d", got, trendAllPageDays)
	}
	if dailyAll.Offset != 0 || !dailyAll.HasOlder {
		t.Fatalf("dailyAll pagination = (%d,%t), want (0,true)", dailyAll.Offset, dailyAll.HasOlder)
	}
	if dailyAll.Labels[0] != "2026-02-24" || dailyAll.Labels[len(dailyAll.Labels)-1] != "2026-03-10" {
		t.Fatalf(
			"dailyAll labels = (%s,%s), want (2026-02-24,2026-03-10)",
			dailyAll.Labels[0],
			dailyAll.Labels[len(dailyAll.Labels)-1],
		)
	}
	assertTrendSeriesValue(t, dailyAll, trendAllModelName, "2026-03-05", 1, 40)
	assertTrendSeriesValue(t, dailyAll, trendAllModelName, "2026-03-10", 5, 83)

	dailyOlderPage, err := stats.TrendContext(context.Background(), TrendOptions{
		Granularity: trendGranularityDay,
		Range:       trendRangeAll,
		Offset:      trendAllPageDays,
		Now:         now,
		Models:      []string{trendAllModelName, "model-old"},
	})
	if err != nil {
		t.Fatalf("daily older trend: %v", err)
	}
	if dailyOlderPage.Offset != trendAllPageDays || dailyOlderPage.HasOlder {
		t.Fatalf("dailyOlderPage pagination = (%d,%t), want (%d,false)", dailyOlderPage.Offset, dailyOlderPage.HasOlder, trendAllPageDays)
	}
	if dailyOlderPage.Labels[0] != "2026-02-09" || dailyOlderPage.Labels[len(dailyOlderPage.Labels)-1] != "2026-02-23" {
		t.Fatalf(
			"dailyOlderPage labels = (%s,%s), want (2026-02-09,2026-02-23)",
			dailyOlderPage.Labels[0],
			dailyOlderPage.Labels[len(dailyOlderPage.Labels)-1],
		)
	}
	assertTrendSeriesValue(t, dailyOlderPage, trendAllModelName, "2026-02-20", 1, 99)
	assertTrendSeriesValue(t, dailyOlderPage, "model-old", "2026-02-20", 1, 99)
}

func TestRequestStatistics_TrendModelsContextSQLite(t *testing.T) {
	original := StatisticsEnabled()
	SetStatisticsEnabled(true)
	t.Cleanup(func() { SetStatisticsEnabled(original) })

	stats := NewRequestStatistics()
	if err := stats.Configure(t.TempDir()); err != nil {
		t.Fatalf("configure stats: %v", err)
	}
	t.Cleanup(func() { _ = stats.Close() })

	now := time.Date(2026, 3, 10, 12, 34, 0, 0, time.UTC)
	records := []coreusage.Record{
		{
			APIKey:      "api-models",
			Model:       "model-a",
			RequestedAt: time.Date(2026, 3, 10, 12, 5, 0, 0, time.UTC),
			Detail:      coreusage.Detail{TotalTokens: 15},
		},
		{
			APIKey:      "api-models",
			Model:       "model-a",
			RequestedAt: time.Date(2026, 3, 10, 10, 10, 0, 0, time.UTC),
			Detail:      coreusage.Detail{TotalTokens: 7},
		},
		{
			APIKey:      "api-models",
			Model:       "model-c",
			RequestedAt: time.Date(2026, 3, 10, 8, 0, 0, 0, time.UTC),
			Detail:      coreusage.Detail{TotalTokens: 30},
		},
		{
			APIKey:      "api-models",
			Model:       "model-b",
			RequestedAt: time.Date(2026, 3, 10, 12, 15, 0, 0, time.UTC),
			Detail:      coreusage.Detail{TotalTokens: 20},
		},
		{
			APIKey:      "api-models",
			Model:       "model-old",
			RequestedAt: time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC),
			Detail:      coreusage.Detail{TotalTokens: 99},
		},
	}
	for _, record := range records {
		stats.Record(context.Background(), record)
	}

	snapshot, err := stats.TrendModelsContext(context.Background(), TrendModelsOptions{
		Since: now.Add(-24 * time.Hour),
		Now:   now,
		Range: trendRange24h,
	})
	if err != nil {
		t.Fatalf("trend models: %v", err)
	}
	if snapshot.Range != trendRange24h {
		t.Fatalf("snapshot.Range = %q, want %q", snapshot.Range, trendRange24h)
	}
	if got := len(snapshot.Models); got != 3 {
		t.Fatalf("len(snapshot.Models) = %d, want 3", got)
	}
	if snapshot.Models[0].ModelName != "model-a" || snapshot.Models[0].Requests != 2 || snapshot.Models[0].Tokens != 22 {
		t.Fatalf("snapshot.Models[0] = %#v, want model-a with 2 requests and 22 tokens", snapshot.Models[0])
	}
	if snapshot.Models[1].ModelName != "model-c" || snapshot.Models[1].Requests != 1 || snapshot.Models[1].Tokens != 30 {
		t.Fatalf("snapshot.Models[1] = %#v, want model-c with 1 request and 30 tokens", snapshot.Models[1])
	}
	if snapshot.Models[2].ModelName != "model-b" || snapshot.Models[2].Requests != 1 || snapshot.Models[2].Tokens != 20 {
		t.Fatalf("snapshot.Models[2] = %#v, want model-b with 1 request and 20 tokens", snapshot.Models[2])
	}
}

func TestRequestStatistics_TrendContextSQLiteUsesUTCPlus8Buckets(t *testing.T) {
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
			APIKey:      "api-trend-boundary",
			Model:       "model-a",
			RequestedAt: time.Date(2026, 3, 9, 15, 59, 59, 0, time.UTC),
			Detail:      coreusage.Detail{TotalTokens: 5},
		},
		{
			APIKey:      "api-trend-boundary",
			Model:       "model-a",
			RequestedAt: time.Date(2026, 3, 9, 16, 0, 0, 0, time.UTC),
			Detail:      coreusage.Detail{TotalTokens: 7},
		},
	}
	for _, record := range records {
		stats.Record(context.Background(), record)
	}

	hourly, err := stats.TrendContext(context.Background(), TrendOptions{
		Granularity: trendGranularityHour,
		Range:       trendRange24h,
		Now:         now,
		Models:      []string{trendAllModelName},
	})
	if err != nil {
		t.Fatalf("hourly boundary trend: %v", err)
	}
	assertTrendSeriesValue(t, hourly, trendAllModelName, "03-09 23:00", 1, 5)
	assertTrendSeriesValue(t, hourly, trendAllModelName, "03-10 00:00", 1, 7)

	daily, err := stats.TrendContext(context.Background(), TrendOptions{
		Granularity: trendGranularityDay,
		Range:       trendRange24h,
		Now:         now,
		Models:      []string{trendAllModelName},
	})
	if err != nil {
		t.Fatalf("daily boundary trend: %v", err)
	}
	assertTrendSeriesValue(t, daily, trendAllModelName, "2026-03-10", 1, 7)
}

func assertTrendSeriesValue(
	t *testing.T,
	snapshot TrendSnapshot,
	modelName string,
	label string,
	requests int64,
	tokens int64,
) {
	t.Helper()

	labelIndex := -1
	for index, candidate := range snapshot.Labels {
		if candidate == label {
			labelIndex = index
			break
		}
	}
	if labelIndex < 0 {
		t.Fatalf("label %s not found", label)
	}

	for _, series := range snapshot.Series {
		if series.ModelName != modelName {
			continue
		}
		if labelIndex >= len(series.Requests) || labelIndex >= len(series.Tokens) {
			t.Fatalf("series %s missing label index %d", modelName, labelIndex)
		}
		if series.Requests[labelIndex] != requests || series.Tokens[labelIndex] != tokens {
			t.Fatalf(
				"series %s at %s = (%d,%d), want (%d,%d)",
				modelName,
				label,
				series.Requests[labelIndex],
				series.Tokens[labelIndex],
				requests,
				tokens,
			)
		}
		return
	}

	t.Fatalf("series %s not found", modelName)
}
