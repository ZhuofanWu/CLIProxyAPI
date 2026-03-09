package usage

import (
	"context"
	"math"
	"testing"
	"time"

	coreusage "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/usage"
)

func TestRequestStatistics_GeneralContextSQLite(t *testing.T) {
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
			APIKey:      "api-general",
			Model:       "model-a",
			RequestedAt: now.Add(-50 * time.Minute),
			Detail: coreusage.Detail{
				InputTokens:     4_000_000,
				OutputTokens:    1_000_000,
				ReasoningTokens: 200_000,
				CachedTokens:    1_000_000,
				TotalTokens:     6_200_000,
			},
		},
		{
			APIKey:      "api-general",
			Model:       "model-a",
			RequestedAt: now.Add(-20 * time.Minute),
			Failed:      true,
			Detail: coreusage.Detail{
				InputTokens:  2_000_000,
				OutputTokens: 2_000_000,
				CachedTokens: 500_000,
				TotalTokens:  4_500_000,
			},
		},
		{
			APIKey:      "api-general",
			Model:       "model-b",
			RequestedAt: now.Add(-5 * time.Minute),
			Detail: coreusage.Detail{
				InputTokens:     1_000_000,
				OutputTokens:    3_000_000,
				ReasoningTokens: 100_000,
				TotalTokens:     4_100_000,
			},
		},
	}
	for _, record := range records {
		stats.Record(context.Background(), record)
	}

	general, err := stats.GeneralContext(context.Background(), GeneralOptions{
		Since: now.Add(-24 * time.Hour),
		Now:   now,
		ModelPrices: map[string]ModelPrice{
			"model-a": {PromptMilli: 1000, CompletionMilli: 2000, CacheMilli: 500},
			"model-b": {PromptMilli: 2000, CompletionMilli: 1000, CacheMilli: 1000},
		},
	})
	if err != nil {
		t.Fatalf("GeneralContext: %v", err)
	}

	if general.Summary.TotalRequests != 3 {
		t.Fatalf("TotalRequests = %d, want 3", general.Summary.TotalRequests)
	}
	if general.Summary.SuccessCount != 2 {
		t.Fatalf("SuccessCount = %d, want 2", general.Summary.SuccessCount)
	}
	if general.Summary.FailureCount != 1 {
		t.Fatalf("FailureCount = %d, want 1", general.Summary.FailureCount)
	}
	if general.Summary.TotalTokens != 14_800_000 {
		t.Fatalf("TotalTokens = %d, want 14800000", general.Summary.TotalTokens)
	}
	if general.Summary.CachedTokens != 1_500_000 {
		t.Fatalf("CachedTokens = %d, want 1500000", general.Summary.CachedTokens)
	}
	if general.Summary.ReasoningTokens != 300_000 {
		t.Fatalf("ReasoningTokens = %d, want 300000", general.Summary.ReasoningTokens)
	}
	if general.Summary.RPMRequestCount30m != 2 {
		t.Fatalf("RPMRequestCount30m = %d, want 2", general.Summary.RPMRequestCount30m)
	}
	if general.Summary.TPMTokenCount30m != 8_600_000 {
		t.Fatalf("TPMTokenCount30m = %d, want 8600000", general.Summary.TPMTokenCount30m)
	}
	if diff := math.Abs(general.Summary.RPM30m - (2.0 / 30.0)); diff > 1e-9 {
		t.Fatalf("RPM30m = %.12f, want %.12f", general.Summary.RPM30m, 2.0/30.0)
	}
	if diff := math.Abs(general.Summary.TPM30m - (8_600_000.0 / 30.0)); diff > 1e-6 {
		t.Fatalf("TPM30m = %.6f, want %.6f", general.Summary.TPM30m, 8_600_000.0/30.0)
	}
	if !general.Summary.CostAvailable {
		t.Fatalf("CostAvailable = false, want true")
	}
	if diff := math.Abs(general.Summary.TotalCost - 16.25); diff > 1e-9 {
		t.Fatalf("TotalCost = %.9f, want 16.25", general.Summary.TotalCost)
	}

	if got := len(general.Series.Requests60m); got != 60 {
		t.Fatalf("len(Requests60m) = %d, want 60", got)
	}
	if got := len(general.Series.Tokens60m); got != 60 {
		t.Fatalf("len(Tokens60m) = %d, want 60", got)
	}
	if got := len(general.Series.RPM30m); got != 30 {
		t.Fatalf("len(RPM30m) = %d, want 30", got)
	}
	if got := len(general.Series.TPM30m); got != 30 {
		t.Fatalf("len(TPM30m) = %d, want 30", got)
	}
	if got := len(general.Series.Cost30m); got != 30 {
		t.Fatalf("len(Cost30m) = %d, want 30", got)
	}

	if general.Series.Requests60m[10].Value != 1 {
		t.Fatalf("Requests60m[10] = %.0f, want 1", general.Series.Requests60m[10].Value)
	}
	if general.Series.Tokens60m[10].Value != 6_200_000 {
		t.Fatalf("Tokens60m[10] = %.0f, want 6200000", general.Series.Tokens60m[10].Value)
	}
	if general.Series.Requests60m[40].Value != 1 {
		t.Fatalf("Requests60m[40] = %.0f, want 1", general.Series.Requests60m[40].Value)
	}
	if general.Series.Tokens60m[55].Value != 4_100_000 {
		t.Fatalf("Tokens60m[55] = %.0f, want 4100000", general.Series.Tokens60m[55].Value)
	}
	if general.Series.RPM30m[10].Value != 1 {
		t.Fatalf("RPM30m[10] = %.0f, want 1", general.Series.RPM30m[10].Value)
	}
	if general.Series.TPM30m[10].Value != 4_500_000 {
		t.Fatalf("TPM30m[10] = %.0f, want 4500000", general.Series.TPM30m[10].Value)
	}
	if diff := math.Abs(general.Series.Cost30m[10].Value - 5.75); diff > 1e-9 {
		t.Fatalf("Cost30m[10] = %.9f, want 5.75", general.Series.Cost30m[10].Value)
	}
	if diff := math.Abs(general.Series.Cost30m[25].Value - 5.0); diff > 1e-9 {
		t.Fatalf("Cost30m[25] = %.9f, want 5.0", general.Series.Cost30m[25].Value)
	}
}
