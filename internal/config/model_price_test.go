package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfig_ModelPrice(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")
	content := strings.Join([]string{
		"host: \"\"",
		"model_price:",
		"  - name: \"gpt-4o\"",
		"    input: 5",
		"    output: 15.125",
		"    cache_read: 2.5",
	}, "\n")
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if len(cfg.ModelPrice) != 1 {
		t.Fatalf("expected 1 model price item, got %d", len(cfg.ModelPrice))
	}
	item := cfg.ModelPrice[0]
	if item.Name != "gpt-4o" {
		t.Fatalf("expected name gpt-4o, got %q", item.Name)
	}
	if got := item.Input.String(); got != "5.000" {
		t.Fatalf("expected input 5.000, got %s", got)
	}
	if got := item.Output.String(); got != "15.125" {
		t.Fatalf("expected output 15.125, got %s", got)
	}
	if got := item.CacheRead.String(); got != "2.500" {
		t.Fatalf("expected cache_read 2.500, got %s", got)
	}
}

func TestLoadConfig_ModelPriceRejectsTooManyDecimals(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")
	content := strings.Join([]string{
		"host: \"\"",
		"model_price:",
		"  - name: \"gpt-4o\"",
		"    input: 5.0001",
		"    output: 15.000",
		"    cache_read: 2.500",
	}, "\n")
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := LoadConfig(configPath)
	if err == nil {
		t.Fatal("expected LoadConfig to reject more than 3 decimal places")
	}
	if !strings.Contains(err.Error(), "at most 3 decimal places") {
		t.Fatalf("expected decimal precision error, got %v", err)
	}
}

func TestSaveConfigPreserveComments_ModelPriceUsesThreeDecimals(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")
	content := strings.Join([]string{
		"host: \"\"",
		"model_price:",
		"  - name: \"gpt-4o\"",
		"    input: 5",
		"    output: 15",
		"    cache_read: 2.5",
	}, "\n")
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if err := SaveConfigPreserveComments(configPath, cfg); err != nil {
		t.Fatalf("SaveConfigPreserveComments: %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	text := string(data)
	for _, expected := range []string{"input: 5.000", "output: 15.000", "cache_read: 2.500"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("expected saved config to contain %q, got:\n%s", expected, text)
		}
	}
}
