package management

import (
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/usage"
)

func (h *Handler) buildUsageModelPrices() map[string]usage.ModelPrice {
	modelPrices := make(map[string]usage.ModelPrice)
	if h == nil || h.cfg == nil {
		return modelPrices
	}
	for _, item := range h.cfg.ModelPrice {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			continue
		}
		modelPrices[name] = usage.ModelPrice{
			PromptMilli:     item.Input.Milli(),
			CompletionMilli: item.Output.Milli(),
			CacheMilli:      item.CacheRead.Milli(),
		}
	}
	return modelPrices
}
