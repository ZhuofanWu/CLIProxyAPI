package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigOptional_DefaultsUsageStatisticsStorageWayToMemory(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte("port: 8080\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfigOptional(configPath, false)
	if err != nil {
		t.Fatalf("LoadConfigOptional: %v", err)
	}
	if cfg.UsageStatisticsStorageWay != UsageStatisticsStorageWayMemory {
		t.Fatalf("UsageStatisticsStorageWay = %q, want %q", cfg.UsageStatisticsStorageWay, UsageStatisticsStorageWayMemory)
	}
}

func TestLoadConfigOptional_RejectsInvalidUsageStatisticsStorageWay(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte("port: 8080\nusage_statistics_storage_way: bad\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if _, err := LoadConfigOptional(configPath, false); err == nil {
		t.Fatal("expected invalid usage_statistics_storage_way to fail")
	}
}
