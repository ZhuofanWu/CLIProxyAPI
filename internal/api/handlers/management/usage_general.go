package management

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/usage"
)

// GetUsageGeneral returns sqlite-only aggregated overview data for the usage page top cards.
func (h *Handler) GetUsageGeneral(c *gin.Context) {
	if h == nil || h.usageStats == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "usage statistics unavailable"})
		return
	}

	snapshotOptions, err := buildUsageSnapshotOptions(c.Query("range"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	modelPrices := make(map[string]usage.ModelPrice)
	if h.cfg != nil {
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
	}

	general, err := h.usageStats.GeneralContext(c.Request.Context(), usage.GeneralOptions{
		Since:       snapshotOptions.Since,
		Now:         time.Now().UTC(),
		ModelPrices: modelPrices,
	})
	if err != nil {
		if errors.Is(err, usage.ErrGeneralUnsupported) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "usage/general is only available when usage statistics storage way is sqlite",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, general)
}
