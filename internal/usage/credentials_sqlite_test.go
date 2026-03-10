package usage

import (
	"context"
	"math"
	"testing"
	"time"

	coreusage "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/usage"
)

func TestRequestStatistics_CredentialsContextSQLite(t *testing.T) {
	original := StatisticsEnabled()
	SetStatisticsEnabled(true)
	t.Cleanup(func() { SetStatisticsEnabled(original) })

	stats := NewRequestStatistics()
	if err := stats.Configure(t.TempDir()); err != nil {
		t.Fatalf("configure stats: %v", err)
	}
	t.Cleanup(func() { _ = stats.Close() })

	now := time.Date(2026, 3, 9, 12, 0, 0, 0, time.UTC)
	records := []coreusage.Record{
		{
			APIKey:      "api-a",
			Model:       "model-a",
			Source:      "source-a",
			AuthIndex:   "auth-1",
			RequestedAt: now,
			Detail:      coreusage.Detail{TotalTokens: 1},
		},
		{
			APIKey:      "api-a",
			Model:       "model-a",
			Source:      "source-a",
			AuthIndex:   "auth-1",
			RequestedAt: now.Add(-5 * time.Minute),
			Failed:      true,
			Detail:      coreusage.Detail{TotalTokens: 1},
		},
		{
			APIKey:      "api-a",
			Model:       "model-a",
			AuthIndex:   "auth-1",
			RequestedAt: now.Add(-20 * time.Minute),
			Detail:      coreusage.Detail{TotalTokens: 1},
		},
		{
			APIKey:      "api-b",
			Model:       "model-b",
			Source:      "source-b",
			AuthIndex:   "auth-2",
			RequestedAt: now.Add(-6 * time.Hour),
			Detail:      coreusage.Detail{TotalTokens: 1},
		},
	}
	for _, record := range records {
		stats.Record(context.Background(), record)
	}

	snapshot, err := stats.CredentialsContext(context.Background(), CredentialsOptions{
		Now:                now,
		Range:              "all",
		IncludePercentData: true,
	})
	if err != nil {
		t.Fatalf("CredentialsContext: %v", err)
	}

	if snapshot.Range != "all" {
		t.Fatalf("Range = %q, want all", snapshot.Range)
	}
	if !snapshot.PercentData {
		t.Fatalf("PercentData = false, want true")
	}
	if len(snapshot.Credentials) != 3 {
		t.Fatalf("len(Credentials) = %d, want 3", len(snapshot.Credentials))
	}

	itemByKey := make(map[string]CredentialUsageItem, len(snapshot.Credentials))
	for _, item := range snapshot.Credentials {
		itemByKey[credentialUsageKey(item.Source, item.AuthIndex)] = item
	}

	sourceA := itemByKey[credentialUsageKey("source-a", "auth-1")]
	if sourceA.Success != 1 || sourceA.Failure != 1 || sourceA.Total != 2 {
		t.Fatalf("source-a counts = (%d,%d,%d), want (1,1,2)", sourceA.Success, sourceA.Failure, sourceA.Total)
	}
	if sourceA.Health == nil {
		t.Fatalf("source-a health = nil, want snapshot")
	}
	if len(sourceA.Health.Rates) != usageCredentialHealthBucketCount {
		t.Fatalf("len(source-a rates) = %d, want %d", len(sourceA.Health.Rates), usageCredentialHealthBucketCount)
	}
	if sourceA.Health.Rows != usageCredentialHealthRows || sourceA.Health.Cols != usageCredentialHealthCols {
		t.Fatalf("source-a rows/cols = %d/%d, want %d/%d", sourceA.Health.Rows, sourceA.Health.Cols, usageCredentialHealthRows, usageCredentialHealthCols)
	}
	if sourceA.Health.Rates[19] != 50 {
		t.Fatalf("source-a rates[19] = %d, want 50", sourceA.Health.Rates[19])
	}
	if sourceA.Health.TotalSuccess != 1 || sourceA.Health.TotalFailure != 1 {
		t.Fatalf("source-a health totals = (%d,%d), want (1,1)", sourceA.Health.TotalSuccess, sourceA.Health.TotalFailure)
	}
	if diff := math.Abs(sourceA.SuccessRate - 50); diff > 1e-9 {
		t.Fatalf("source-a SuccessRate = %.9f, want 50", sourceA.SuccessRate)
	}

	authOnly := itemByKey[credentialUsageKey("", "auth-1")]
	if authOnly.Success != 1 || authOnly.Failure != 0 || authOnly.Total != 1 {
		t.Fatalf("auth-only counts = (%d,%d,%d), want (1,0,1)", authOnly.Success, authOnly.Failure, authOnly.Total)
	}
	if authOnly.Health == nil || authOnly.Health.Rates[18] != 100 {
		t.Fatalf("auth-only rates[18] = %d, want 100", authOnly.Health.Rates[18])
	}

	oldSource := itemByKey[credentialUsageKey("source-b", "auth-2")]
	if oldSource.Success != 1 || oldSource.Failure != 0 || oldSource.Total != 1 {
		t.Fatalf("source-b counts = (%d,%d,%d), want (1,0,1)", oldSource.Success, oldSource.Failure, oldSource.Total)
	}
	if oldSource.Health == nil {
		t.Fatalf("source-b health = nil, want snapshot")
	}
	for index, rate := range oldSource.Health.Rates {
		if rate != -1 {
			t.Fatalf("source-b rates[%d] = %d, want -1", index, rate)
		}
	}

	filtered, err := stats.CredentialsContext(context.Background(), CredentialsOptions{
		Since: now.Add(-30 * time.Minute),
		Now:   now,
		Range: "24h",
	})
	if err != nil {
		t.Fatalf("CredentialsContext filtered: %v", err)
	}
	if len(filtered.Credentials) != 2 {
		t.Fatalf("len(filtered.Credentials) = %d, want 2", len(filtered.Credentials))
	}
}
