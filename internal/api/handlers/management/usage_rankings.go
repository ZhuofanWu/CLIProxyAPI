package management

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/usage"
)

// GetUsageRankings returns sqlite-only aggregated ranking data for the usage page details cards.
func (h *Handler) GetUsageRankings(c *gin.Context) {
	if h == nil || h.usageStats == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "usage statistics unavailable"})
		return
	}

	snapshotOptions, err := buildUsageSnapshotOptions(c.Query("range"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	rankings, err := h.usageStats.RankingsContext(c.Request.Context(), usage.RankingsOptions{
		Since:       snapshotOptions.Since,
		Now:         time.Now().UTC(),
		ModelPrices: h.buildUsageModelPrices(),
	})
	if err != nil {
		if errors.Is(err, usage.ErrRankingsUnsupported) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "usage/rankings is only available when usage statistics storage way is sqlite",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, rankings)
}
