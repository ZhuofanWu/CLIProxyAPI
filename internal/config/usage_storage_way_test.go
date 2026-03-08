package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigOptional_DefaultsUsageStorageWayToMemory(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte("port: 8080\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfigOptional(configPath, false)
	if err != nil {
		t.Fatalf("LoadConfigOptional: %v", err)
	}
	if cfg.UsageStaticStorageWay != UsageStaticStorageWayMemory {
		t.Fatalf("UsageStaticStorageWay = %q, want %q", cfg.UsageStaticStorageWay, UsageStaticStorageWayMemory)
	}
}

func TestLoadConfigOptional_RejectsInvalidUsageStorageWay(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte("port: 8080\nusage_static_storage_way: bad\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if _, err := LoadConfigOptional(configPath, false); err == nil {
		t.Fatal("expected invalid usage_static_storage_way to fail")
	}
}
