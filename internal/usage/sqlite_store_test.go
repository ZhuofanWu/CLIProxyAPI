package usage

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	coreusage "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/usage"
)

func TestRequestStatistics_PersistedSnapshot(t *testing.T) {
	original := StatisticsEnabled()
	SetStatisticsEnabled(true)
	t.Cleanup(func() { SetStatisticsEnabled(original) })

	stats := NewRequestStatistics()
	if err := stats.Configure(t.TempDir()); err != nil {
		t.Fatalf("configure stats: %v", err)
	}
	t.Cleanup(func() { _ = stats.Close() })

	ts1 := time.Date(2026, 3, 7, 8, 15, 0, 0, time.UTC)
	ts2 := ts1.Add(30 * time.Minute)
	ts3 := ts1.Add(2 * time.Hour)

	stats.Record(context.Background(), coreusage.Record{
		Provider:    "openai",
		APIKey:      "api-1",
		Model:       "gpt-4.1",
		RequestedAt: ts1,
		Source:      "alice@example.com",
		AuthIndex:   "0",
		Detail: coreusage.Detail{
			InputTokens:     10,
			OutputTokens:    20,
			ReasoningTokens: 3,
			CachedTokens:    5,
			TotalTokens:     33,
		},
	})
	stats.Record(context.Background(), coreusage.Record{
		Provider:    "openai",
		APIKey:      "api-1",
		Model:       "gpt-4.1",
		RequestedAt: ts2,
		Source:      "alice@example.com",
		AuthIndex:   "0",
		Detail: coreusage.Detail{
			InputTokens:     6,
			OutputTokens:    4,
			ReasoningTokens: 1,
			CachedTokens:    2,
			TotalTokens:     11,
		},
	})
	stats.Record(context.Background(), coreusage.Record{
		Provider:    "anthropic",
		APIKey:      "api-2",
		Model:       "claude-sonnet",
		RequestedAt: ts3,
		Source:      "bob@example.com",
		AuthIndex:   "1",
		Failed:      true,
	})

	snapshot, err := stats.SnapshotContext(context.Background())
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if snapshot.TotalRequests != 3 {
		t.Fatalf("total requests = %d, want 3", snapshot.TotalRequests)
	}
	if snapshot.SuccessCount != 2 {
		t.Fatalf("success count = %d, want 2", snapshot.SuccessCount)
	}
	if snapshot.FailureCount != 1 {
		t.Fatalf("failure count = %d, want 1", snapshot.FailureCount)
	}
	if snapshot.TotalTokens != 44 {
		t.Fatalf("total tokens = %d, want 44", snapshot.TotalTokens)
	}
	if snapshot.RequestsByDay["2026-03-07"] != 3 {
		t.Fatalf("requests_by_day mismatch: %#v", snapshot.RequestsByDay)
	}
	if snapshot.RequestsByHour["08"] != 2 || snapshot.RequestsByHour["10"] != 1 {
		t.Fatalf("requests_by_hour mismatch: %#v", snapshot.RequestsByHour)
	}
	api1 := snapshot.APIs["api-1"]
	if api1.TotalRequests != 2 || api1.TotalTokens != 44 {
		t.Fatalf("api-1 aggregate mismatch: %#v", api1)
	}
	model := api1.Models["gpt-4.1"]
	if model.InputTokens != 16 || model.OutputTokens != 24 || model.ReasoningTokens != 4 || model.CachedTokens != 7 {
		t.Fatalf("model token breakdown mismatch: %#v", model)
	}
	if len(model.Details) != 2 {
		t.Fatalf("model details count = %d, want 2", len(model.Details))
	}
	if !model.Details[0].Timestamp.Equal(ts1) || !model.Details[1].Timestamp.Equal(ts2) {
		t.Fatalf("model details timestamps mismatch: %#v", model.Details)
	}
	if model.Details[0].Source != "alice@example.com" || model.Details[0].AuthIndex != "0" {
		t.Fatalf("first model detail mismatch: %#v", model.Details[0])
	}
	if model.Details[0].Tokens.InputTokens != 10 || model.Details[1].Tokens.OutputTokens != 4 {
		t.Fatalf("model detail token breakdown mismatch: %#v", model.Details)
	}
	failedModel := snapshot.APIs["api-2"].Models["claude-sonnet"]
	if len(failedModel.Details) != 1 || !failedModel.Details[0].Failed {
		t.Fatalf("failed model details mismatch: %#v", failedModel)
	}

	dbPath := stats.DatabasePath()
	if dbPath == "" {
		t.Fatal("expected database path to be configured")
	}
	if err := stats.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := stats.Configure(filepath.Dir(dbPath)); err != nil {
		t.Fatalf("reconfigure: %v", err)
	}

	snapshot, err = stats.SnapshotContext(context.Background())
	if err != nil {
		t.Fatalf("snapshot after reopen: %v", err)
	}
	if snapshot.TotalRequests != 3 || snapshot.TotalTokens != 44 {
		t.Fatalf("snapshot after reopen mismatch: %#v", snapshot)
	}
	if got := len(snapshot.APIs["api-1"].Models["gpt-4.1"].Details); got != 2 {
		t.Fatalf("snapshot after reopen details count = %d, want 2", got)
	}
}

func TestRequestStatistics_SnapshotContextWithOptionsFiltersRangeAndLimitsDetails(t *testing.T) {
	original := StatisticsEnabled()
	SetStatisticsEnabled(true)
	t.Cleanup(func() { SetStatisticsEnabled(original) })

	stats := NewRequestStatistics()
	if err := stats.Configure(t.TempDir()); err != nil {
		t.Fatalf("configure stats: %v", err)
	}
	t.Cleanup(func() { _ = stats.Close() })

	base := time.Date(2026, 3, 7, 0, 0, 0, 0, time.UTC)
	stats.Record(context.Background(), coreusage.Record{
		Provider:    "openai",
		APIKey:      "api-range",
		Model:       "too-old",
		RequestedAt: base.Add(-time.Hour),
		Detail: coreusage.Detail{
			InputTokens:  1,
			OutputTokens: 1,
			TotalTokens:  2,
		},
	})

	for index := 0; index < 120; index++ {
		model := "model-a"
		if index%2 == 1 {
			model = "model-b"
		}
		stats.Record(context.Background(), coreusage.Record{
			Provider:    "openai",
			APIKey:      "api-range",
			Model:       model,
			RequestedAt: base.Add(time.Duration(index) * time.Minute),
			Detail: coreusage.Detail{
				InputTokens:  2,
				OutputTokens: 3,
				TotalTokens:  5,
			},
		})
	}

	snapshot, err := stats.SnapshotContextWithOptions(context.Background(), SnapshotOptions{
		Since:       base,
		DetailLimit: 100,
	})
	if err != nil {
		t.Fatalf("snapshot with options: %v", err)
	}
	if snapshot.TotalRequests != 120 {
		t.Fatalf("filtered total requests = %d, want 120", snapshot.TotalRequests)
	}
	if snapshot.TotalTokens != 600 {
		t.Fatalf("filtered total tokens = %d, want 600", snapshot.TotalTokens)
	}
	apiSnapshot := snapshot.APIs["api-range"]
	if apiSnapshot.TotalRequests != 120 {
		t.Fatalf("api-range total requests = %d, want 120", apiSnapshot.TotalRequests)
	}

	detailCount := 0
	var oldestIncluded time.Time
	for _, modelSnapshot := range apiSnapshot.Models {
		detailCount += len(modelSnapshot.Details)
		for _, detail := range modelSnapshot.Details {
			if oldestIncluded.IsZero() || detail.Timestamp.Before(oldestIncluded) {
				oldestIncluded = detail.Timestamp
			}
			if detail.Timestamp.Before(base) {
				t.Fatalf("found out-of-range detail: %s", detail.Timestamp)
			}
		}
	}
	if detailCount != 100 {
		t.Fatalf("detail count = %d, want 100", detailCount)
	}
	wantOldestIncluded := base.Add(20 * time.Minute)
	if !oldestIncluded.Equal(wantOldestIncluded) {
		t.Fatalf("oldest included detail = %s, want %s", oldestIncluded, wantOldestIncluded)
	}
}

func TestRequestStatistics_ExportImportAndLegacyMerge(t *testing.T) {
	original := StatisticsEnabled()
	SetStatisticsEnabled(true)
	t.Cleanup(func() { SetStatisticsEnabled(original) })

	producer := NewRequestStatistics()
	if err := producer.Configure(t.TempDir()); err != nil {
		t.Fatalf("configure producer: %v", err)
	}
	t.Cleanup(func() { _ = producer.Close() })

	ts := time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)
	producer.Record(context.Background(), coreusage.Record{
		Provider:    "openai",
		APIKey:      "api-export",
		Model:       "gpt-4.1-mini",
		RequestedAt: ts,
		Source:      "export@example.com",
		AuthIndex:   "7",
		Detail: coreusage.Detail{
			InputTokens:  8,
			OutputTokens: 12,
			TotalTokens:  20,
		},
	})

	records, err := producer.ExportRecords(context.Background())
	if err != nil {
		t.Fatalf("export records: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("exported records = %d, want 1", len(records))
	}

	consumer := NewRequestStatistics()
	if err := consumer.Configure(t.TempDir()); err != nil {
		t.Fatalf("configure consumer: %v", err)
	}
	t.Cleanup(func() { _ = consumer.Close() })

	result, err := consumer.MergeRecords(context.Background(), records)
	if err != nil {
		t.Fatalf("merge records: %v", err)
	}
	if result.Added != 1 || result.Skipped != 0 {
		t.Fatalf("first merge result = %#v, want added=1 skipped=0", result)
	}
	result, err = consumer.MergeRecords(context.Background(), records)
	if err != nil {
		t.Fatalf("merge duplicate records: %v", err)
	}
	if result.Added != 0 || result.Skipped != 1 {
		t.Fatalf("duplicate merge result = %#v, want added=0 skipped=1", result)
	}

	legacy := StatisticsSnapshot{
		APIs: map[string]APISnapshot{
			"legacy-api": {
				Models: map[string]ModelSnapshot{
					"legacy-model": {
						Details: []RequestDetail{{
							Timestamp: ts.Add(time.Hour),
							Source:    "legacy@example.com",
							AuthIndex: "9",
							Tokens: TokenStats{
								InputTokens:  3,
								OutputTokens: 4,
								TotalTokens:  7,
							},
						}},
					},
				},
			},
		},
	}
	result, err = consumer.MergeSnapshotContext(context.Background(), legacy)
	if err != nil {
		t.Fatalf("merge legacy snapshot: %v", err)
	}
	if result.Added != 1 || result.Skipped != 0 {
		t.Fatalf("legacy merge result = %#v, want added=1 skipped=0", result)
	}

	snapshot, err := consumer.SnapshotContext(context.Background())
	if err != nil {
		t.Fatalf("consumer snapshot: %v", err)
	}
	if snapshot.TotalRequests != 2 || snapshot.TotalTokens != 27 {
		t.Fatalf("consumer snapshot mismatch: %#v", snapshot)
	}
	if snapshot.APIs["legacy-api"].Models["legacy-model"].TotalTokens != 7 {
		t.Fatalf("legacy model aggregate mismatch: %#v", snapshot.APIs["legacy-api"])
	}
}

func TestOpenUsageDatabase_ConfiguresBusyTimeout(t *testing.T) {
	stats := NewRequestStatistics()
	if err := stats.Configure(t.TempDir()); err != nil {
		t.Fatalf("configure stats: %v", err)
	}
	t.Cleanup(func() { _ = stats.Close() })

	db, err := openUsageDatabase(stats.DatabasePath())
	if err != nil {
		t.Fatalf("open usage database: %v", err)
	}
	defer db.Close()

	var timeout int
	if err := db.QueryRow("PRAGMA busy_timeout;").Scan(&timeout); err != nil {
		t.Fatalf("query busy_timeout: %v", err)
	}
	if timeout != 5000 {
		t.Fatalf("busy_timeout = %d, want 5000", timeout)
	}
}

func TestOpenUsageDatabase_DoesNotFailWhenAnotherConnectionHoldsWriteLock(t *testing.T) {
	stats := NewRequestStatistics()
	if err := stats.Configure(t.TempDir()); err != nil {
		t.Fatalf("configure stats: %v", err)
	}
	t.Cleanup(func() { _ = stats.Close() })

	writerDB, err := openUsageDatabase(stats.DatabasePath())
	if err != nil {
		t.Fatalf("open writer database: %v", err)
	}
	defer writerDB.Close()

	if _, err := writerDB.Exec(`BEGIN IMMEDIATE;`); err != nil {
		t.Fatalf("begin immediate: %v", err)
	}
	defer func() {
		_, _ = writerDB.Exec(`ROLLBACK;`)
	}()

	readerDB, err := openUsageDatabase(stats.DatabasePath())
	if err != nil {
		t.Fatalf("open second database while locked: %v", err)
	}
	defer readerDB.Close()

	var timeout int
	if err := readerDB.QueryRow(`PRAGMA busy_timeout;`).Scan(&timeout); err != nil {
		t.Fatalf("query second connection busy_timeout: %v", err)
	}
	if timeout != 5000 {
		t.Fatalf("second connection busy_timeout = %d, want 5000", timeout)
	}
}

func TestRequestStatistics_RecordIgnoresCanceledRequestContext(t *testing.T) {
	original := StatisticsEnabled()
	SetStatisticsEnabled(true)
	t.Cleanup(func() { SetStatisticsEnabled(original) })

	stats := NewRequestStatistics()
	if err := stats.Configure(t.TempDir()); err != nil {
		t.Fatalf("configure stats: %v", err)
	}
	t.Cleanup(func() { _ = stats.Close() })

	requestCtx, cancel := context.WithCancel(context.Background())
	cancel()

	stats.Record(requestCtx, coreusage.Record{
		Provider:    "openai",
		APIKey:      "api-canceled",
		Model:       "gpt-4.1",
		RequestedAt: time.Date(2026, 3, 7, 10, 28, 1, 0, time.UTC),
		Detail: coreusage.Detail{
			InputTokens:  11,
			OutputTokens: 13,
			TotalTokens:  24,
		},
	})

	snapshot, err := stats.SnapshotContext(context.Background())
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if snapshot.TotalRequests != 1 {
		t.Fatalf("total requests = %d, want 1", snapshot.TotalRequests)
	}
	if snapshot.TotalTokens != 24 {
		t.Fatalf("total tokens = %d, want 24", snapshot.TotalTokens)
	}
	if snapshot.APIs["api-canceled"].Models["gpt-4.1"].TotalRequests != 1 {
		t.Fatalf("api/model aggregate missing: %#v", snapshot.APIs)
	}
}
