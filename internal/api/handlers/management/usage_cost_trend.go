package management

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/usage"
)

// GetUsageCostTrend returns sqlite-only cost trend buckets for the usage page.
func (h *Handler) GetUsageCostTrend(c *gin.Context) {
	if h == nil || h.usageStats == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "usage statistics unavailable"})
		return
	}

	options, err := buildCostTrendOptions(
		c.Query("granularity"),
		c.Query("range"),
		c.Query("offset"),
	)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	snapshot, err := h.usageStats.CostTrendContext(c.Request.Context(), usage.CostTrendOptions{
		Granularity: options.Granularity,
		Range:       options.Range,
		Offset:      options.Offset,
		Now:         options.Now,
		ModelPrices: h.buildUsageModelPrices(),
	})
	if err != nil {
		if errors.Is(err, usage.ErrCostTrendUnsupported) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "usage/cost-trend is only available when usage statistics storage way is sqlite",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, snapshot)
}

func buildCostTrendOptions(
	rawGranularity string,
	rawRange string,
	rawOffset string,
) (usage.CostTrendOptions, error) {
	tokenOptions, err := buildTokenBreakdownOptions(rawGranularity, rawRange, rawOffset)
	if err != nil {
		return usage.CostTrendOptions{}, err
	}
	return usage.CostTrendOptions{
		Granularity: tokenOptions.Granularity,
		Range:       tokenOptions.Range,
		Offset:      tokenOptions.Offset,
		Now:         tokenOptions.Now,
	}, nil
}
