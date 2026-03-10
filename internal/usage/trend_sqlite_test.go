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
			Detail:      coreusage.Detail{InputTokens: 5, OutputTokens: 10, TotalTokens: 15},
		},
		{
			APIKey:      "api-trend",
			Model:       "model-a",
			RequestedAt: time.Date(2026, 3, 10, 10, 10, 0, 0, time.UTC),
			Detail:      coreusage.Detail{InputTokens: 3, OutputTokens: 4, TotalTokens: 7},
		},
		{
			APIKey:      "api-trend",
			Model:       "model-b",
			RequestedAt: time.Date(2026, 3, 10, 12, 15, 0, 0, time.UTC),
			Detail:      coreusage.Detail{InputTokens: 8, OutputTokens: 12, TotalTokens: 20},
		},
		{
			APIKey:      "api-trend",
			Model:       "model-c",
			RequestedAt: time.Date(2026, 3, 10, 8, 0, 0, 0, time.UTC),
			Detail:      coreusage.Detail{InputTokens: 12, OutputTokens: 18, TotalTokens: 30},
		},
		{
			APIKey:      "api-trend",
			Model:       "model-d",
			RequestedAt: time.Date(2026, 3, 9, 23, 0, 0, 0, time.UTC),
			Detail:      coreusage.Detail{InputTokens: 5, OutputTokens: 6, TotalTokens: 11},
		},
	}
	for _, record := range records {
		stats.Record(context.Background(), record)
	}

	hourly, err := stats.TrendContext(context.Background(), TrendOptions{
		Granularity: trendGranularityHour,
		Range:       trendRange7h,
		Now:         now,
		Models:      []string{trendAllModelName, "model-a", "model-c"},
	})
	if err != nil {
		t.Fatalf("hourly trend: %v", err)
	}
	if got := len(hourly.Labels); got != 7 {
		t.Fatalf("len(hourly.Labels) = %d, want 7", got)
	}
	if got := len(hourly.Series); got != 3 {
		t.Fatalf("len(hourly.Series) = %d, want 3", got)
	}
	assertTrendSeriesValue(t, hourly, trendAllModelName, "03-10 08:00", 1, 30)
	assertTrendSeriesValue(t, hourly, trendAllModelName, "03-10 10:00", 1, 7)
	assertTrendSeriesValue(t, hourly, trendAllModelName, "03-10 12:00", 2, 35)
	assertTrendSeriesValue(t, hourly, "model-a", "03-10 10:00", 1, 7)
	assertTrendSeriesValue(t, hourly, "model-a", "03-10 12:00", 1, 15)
	assertTrendSeriesValue(t, hourly, "model-c", "03-10 08:00", 1, 30)

	daily, err := stats.TrendContext(context.Background(), TrendOptions{
		Granularity: trendGranularityDay,
		Range:       trendRange24h,
		Now:         now,
		Models:      []string{trendAllModelName, "model-d"},
	})
	if err != nil {
		t.Fatalf("daily trend: %v", err)
	}
	if got := len(daily.Labels); got != 2 {
		t.Fatalf("len(daily.Labels) = %d, want 2", got)
	}
	if daily.Labels[0] != "2026-03-09" || daily.Labels[1] != "2026-03-10" {
		t.Fatalf("daily labels = %#v, want [2026-03-09 2026-03-10]", daily.Labels)
	}
	assertTrendSeriesValue(t, daily, trendAllModelName, "2026-03-09", 1, 11)
	assertTrendSeriesValue(t, daily, trendAllModelName, "2026-03-10", 4, 72)
	assertTrendSeriesValue(t, daily, "model-d", "2026-03-09", 1, 11)
	assertTrendSeriesValue(t, daily, "model-d", "2026-03-10", 0, 0)
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
