package config

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// ModelPriceItem describes per-model pricing configured under model_price.
type ModelPriceItem struct {
	Name      string          `yaml:"name" json:"name"`
	Input     ModelPriceValue `yaml:"input" json:"input"`
	Output    ModelPriceValue `yaml:"output" json:"output"`
	CacheRead ModelPriceValue `yaml:"cache_read" json:"cache_read"`
}

// ModelPriceValue stores a non-negative decimal with up to three fractional digits.
type ModelPriceValue struct {
	milli int64
}

func (m *ModelPriceItem) UnmarshalYAML(node *yaml.Node) error {
	var raw struct {
		Name      *string          `yaml:"name"`
		Input     *ModelPriceValue `yaml:"input"`
		Output    *ModelPriceValue `yaml:"output"`
		CacheRead *ModelPriceValue `yaml:"cache_read"`
	}
	if err := node.Decode(&raw); err != nil {
		return err
	}
	if raw.Name == nil || strings.TrimSpace(*raw.Name) == "" {
		return fmt.Errorf("model_price item missing name")
	}
	if raw.Input == nil {
		return fmt.Errorf("model_price item missing input")
	}
	if raw.Output == nil {
		return fmt.Errorf("model_price item missing output")
	}
	if raw.CacheRead == nil {
		return fmt.Errorf("model_price item missing cache_read")
	}
	m.Name = strings.TrimSpace(*raw.Name)
	m.Input = *raw.Input
	m.Output = *raw.Output
	m.CacheRead = *raw.CacheRead
	return nil
}

func (m *ModelPriceItem) UnmarshalJSON(data []byte) error {
	var raw struct {
		Name      *string          `json:"name"`
		Input     *ModelPriceValue `json:"input"`
		Output    *ModelPriceValue `json:"output"`
		CacheRead *ModelPriceValue `json:"cache_read"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if raw.Name == nil || strings.TrimSpace(*raw.Name) == "" {
		return fmt.Errorf("model_price item missing name")
	}
	if raw.Input == nil {
		return fmt.Errorf("model_price item missing input")
	}
	if raw.Output == nil {
		return fmt.Errorf("model_price item missing output")
	}
	if raw.CacheRead == nil {
		return fmt.Errorf("model_price item missing cache_read")
	}
	m.Name = strings.TrimSpace(*raw.Name)
	m.Input = *raw.Input
	m.Output = *raw.Output
	m.CacheRead = *raw.CacheRead
	return nil
}

func (v ModelPriceValue) String() string {
	return fmt.Sprintf("%d.%03d", v.milli/1000, v.milli%1000)
}

func (v ModelPriceValue) Milli() int64 {
	return v.milli
}

func (v ModelPriceValue) MarshalJSON() ([]byte, error) {
	return []byte(v.String()), nil
}

func (v *ModelPriceValue) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" {
		return fmt.Errorf("model price value is required")
	}
	if strings.HasPrefix(trimmed, `"`) {
		var value string
		if err := json.Unmarshal(data, &value); err != nil {
			return err
		}
		trimmed = value
	}
	milli, err := parseModelPriceMilli(trimmed)
	if err != nil {
		return err
	}
	v.milli = milli
	return nil
}

func (v ModelPriceValue) MarshalYAML() (any, error) {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!float", Value: v.String()}, nil
}

func (v *ModelPriceValue) UnmarshalYAML(node *yaml.Node) error {
	if node == nil {
		return fmt.Errorf("model price value is required")
	}
	milli, err := parseModelPriceMilli(node.Value)
	if err != nil {
		return err
	}
	v.milli = milli
	return nil
}

func parseModelPriceMilli(raw string) (int64, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, fmt.Errorf("model price value is required")
	}
	if strings.ContainsAny(value, "eE") {
		return 0, fmt.Errorf("model price value %q must not use scientific notation", value)
	}
	if strings.HasPrefix(value, "+") {
		value = value[1:]
	}
	if strings.HasPrefix(value, "-") {
		return 0, fmt.Errorf("model price value %q must be non-negative", value)
	}
	parts := strings.Split(value, ".")
	if len(parts) > 2 {
		return 0, fmt.Errorf("invalid model price value %q", value)
	}
	intPart := parts[0]
	if intPart == "" {
		intPart = "0"
	}
	if !digitsOnly(intPart) {
		return 0, fmt.Errorf("invalid model price value %q", value)
	}
	fracPart := ""
	if len(parts) == 2 {
		fracPart = parts[1]
		if fracPart == "" {
			fracPart = "0"
		}
		if !digitsOnly(fracPart) {
			return 0, fmt.Errorf("invalid model price value %q", value)
		}
		if len(fracPart) > 3 {
			return 0, fmt.Errorf("model price value %q must have at most 3 decimal places", value)
		}
	}
	whole, err := strconv.ParseInt(intPart, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid model price value %q: %w", value, err)
	}
	for len(fracPart) < 3 {
		fracPart += "0"
	}
	frac := int64(0)
	if fracPart != "" {
		frac, err = strconv.ParseInt(fracPart, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid model price value %q: %w", value, err)
		}
	}
	return whole*1000 + frac, nil
}

func digitsOnly(value string) bool {
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// SanitizeModelPrice trims model names and removes empty entries.
func (cfg *Config) SanitizeModelPrice() {
	if cfg == nil || len(cfg.ModelPrice) == 0 {
		return
	}
	out := make([]ModelPriceItem, 0, len(cfg.ModelPrice))
	for i := range cfg.ModelPrice {
		item := cfg.ModelPrice[i]
		item.Name = strings.TrimSpace(item.Name)
		if item.Name == "" {
			continue
		}
		out = append(out, item)
	}
	cfg.ModelPrice = out
}
