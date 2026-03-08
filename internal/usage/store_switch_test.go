package usage

import (
	"context"
	"testing"
	"time"

	coreusage "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/usage"
)

func TestRequestStatistics_ConfigureStorageWayMigratesData(t *testing.T) {
	original := StatisticsEnabled()
	SetStatisticsEnabled(true)
	t.Cleanup(func() { SetStatisticsEnabled(original) })

	stats := NewRequestStatistics()
	stats.Record(context.Background(), coreusage.Record{
		APIKey:      "memory-api",
		Model:       "gpt-memory",
		RequestedAt: time.Date(2026, 3, 8, 8, 0, 0, 0, time.UTC),
		Detail: coreusage.Detail{
			InputTokens:  3,
			OutputTokens: 4,
			TotalTokens:  7,
		},
	})

	if stats.StorageWay() != UsageStorageWayMemory {
		t.Fatalf("StorageWay = %q, want %q", stats.StorageWay(), UsageStorageWayMemory)
	}

	if err := stats.ConfigureStorageWay(UsageStorageWaySQLite, t.TempDir()); err != nil {
		t.Fatalf("ConfigureStorageWay sqlite: %v", err)
	}
	if stats.StorageWay() != UsageStorageWaySQLite {
		t.Fatalf("StorageWay = %q, want %q", stats.StorageWay(), UsageStorageWaySQLite)
	}

	snapshot, err := stats.SnapshotContext(context.Background())
	if err != nil {
		t.Fatalf("SnapshotContext sqlite: %v", err)
	}
	if snapshot.TotalRequests != 1 || snapshot.TotalTokens != 7 {
		t.Fatalf("sqlite snapshot mismatch: %#v", snapshot)
	}

	if err := stats.ConfigureStorageWay(UsageStorageWayMemory, ""); err != nil {
		t.Fatalf("ConfigureStorageWay memory: %v", err)
	}
	if stats.StorageWay() != UsageStorageWayMemory {
		t.Fatalf("StorageWay = %q, want %q", stats.StorageWay(), UsageStorageWayMemory)
	}

	snapshot, err = stats.SnapshotContextWithOptions(context.Background(), SnapshotOptions{
		Since:       time.Now().UTC(),
		DetailLimit: 1,
	})
	if err != nil {
		t.Fatalf("SnapshotContextWithOptions memory: %v", err)
	}
	if snapshot.TotalRequests != 1 || len(snapshot.APIs["memory-api"].Models["gpt-memory"].Details) != 1 {
		t.Fatalf("memory snapshot mismatch after switch back: %#v", snapshot)
	}
}
