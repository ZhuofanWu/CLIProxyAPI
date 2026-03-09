package usage

import (
	"context"
	"math"
	"testing"
	"time"

	coreusage "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/usage"
)

func TestRequestStatistics_RankingsContextSQLite(t *testing.T) {
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
			Model:       "model-x",
			RequestedAt: now.Add(-2 * time.Hour),
			Detail: coreusage.Detail{
				InputTokens:  2_000_000,
				OutputTokens: 1_000_000,
				TotalTokens:  3_000_000,
			},
		},
		{
			APIKey:      "api-a",
			Model:       "model-y",
			RequestedAt: now.Add(-90 * time.Minute),
			Failed:      true,
			Detail: coreusage.Detail{
				InputTokens:  1_000_000,
				OutputTokens: 2_000_000,
				TotalTokens:  3_000_000,
			},
		},
		{
			APIKey:      "api-b",
			Model:       "model-x",
			RequestedAt: now.Add(-30 * time.Minute),
			Detail: coreusage.Detail{
				InputTokens:  1_000_000,
				OutputTokens: 1_000_000,
				TotalTokens:  2_000_000,
			},
		},
		{
			APIKey:      "api-b",
			Model:       "model-z",
			RequestedAt: now.Add(-20 * time.Minute),
			Detail: coreusage.Detail{
				InputTokens:  3_000_000,
				OutputTokens: 1_000_000,
				CachedTokens: 1_000_000,
				TotalTokens:  5_000_000,
			},
		},
		{
			APIKey:      "api-b",
			Model:       "model-x",
			RequestedAt: now.Add(-10 * time.Minute),
			Failed:      true,
			Detail: coreusage.Detail{
				InputTokens: 1_000_000,
				TotalTokens: 1_000_000,
			},
		},
	}
	for _, record := range records {
		stats.Record(context.Background(), record)
	}

	rankings, err := stats.RankingsContext(context.Background(), RankingsOptions{
		Since: now.Add(-24 * time.Hour),
		Now:   now,
		ModelPrices: map[string]ModelPrice{
			"model-x": {PromptMilli: 1000, CompletionMilli: 2000, CacheMilli: 500},
			"model-y": {PromptMilli: 2000, CompletionMilli: 1000, CacheMilli: 500},
			"model-z": {PromptMilli: 1000, CompletionMilli: 2000, CacheMilli: 500},
		},
	})
	if err != nil {
		t.Fatalf("RankingsContext: %v", err)
	}

	if got := len(rankings.APIRankings); got != 2 {
		t.Fatalf("len(APIRankings) = %d, want 2", got)
	}
	if got := len(rankings.ModelRankings); got != 3 {
		t.Fatalf("len(ModelRankings) = %d, want 3", got)
	}

	firstAPI := rankings.APIRankings[0]
	if firstAPI.APIName != "api-b" {
		t.Fatalf("first APIName = %q, want api-b", firstAPI.APIName)
	}
	if firstAPI.TotalRequests != 3 {
		t.Fatalf("api-b TotalRequests = %d, want 3", firstAPI.TotalRequests)
	}
	if firstAPI.SuccessCount != 2 {
		t.Fatalf("api-b SuccessCount = %d, want 2", firstAPI.SuccessCount)
	}
	if firstAPI.FailureCount != 1 {
		t.Fatalf("api-b FailureCount = %d, want 1", firstAPI.FailureCount)
	}
	if firstAPI.TotalTokens != 8_000_000 {
		t.Fatalf("api-b TotalTokens = %d, want 8000000", firstAPI.TotalTokens)
	}
	if diff := math.Abs(firstAPI.TotalCost - 8.5); diff > 1e-9 {
		t.Fatalf("api-b TotalCost = %.9f, want 8.5", firstAPI.TotalCost)
	}
	if got := len(firstAPI.Models); got != 2 {
		t.Fatalf("len(api-b Models) = %d, want 2", got)
	}
	if firstAPI.Models[0].ModelName != "model-x" || firstAPI.Models[0].Requests != 2 {
		t.Fatalf("api-b first model = %#v, want model-x with 2 requests", firstAPI.Models[0])
	}

	firstModel := rankings.ModelRankings[0]
	if firstModel.ModelName != "model-x" {
		t.Fatalf("first model = %q, want model-x", firstModel.ModelName)
	}
	if firstModel.Requests != 3 {
		t.Fatalf("model-x Requests = %d, want 3", firstModel.Requests)
	}
	if firstModel.SuccessCount != 2 {
		t.Fatalf("model-x SuccessCount = %d, want 2", firstModel.SuccessCount)
	}
	if firstModel.FailureCount != 1 {
		t.Fatalf("model-x FailureCount = %d, want 1", firstModel.FailureCount)
	}
	if firstModel.Tokens != 6_000_000 {
		t.Fatalf("model-x Tokens = %d, want 6000000", firstModel.Tokens)
	}
	if diff := math.Abs(firstModel.Cost - 8.0); diff > 1e-9 {
		t.Fatalf("model-x Cost = %.9f, want 8.0", firstModel.Cost)
	}
}
