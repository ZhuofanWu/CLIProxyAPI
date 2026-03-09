package management

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/usage"
)

// GetUsageHealth returns sqlite-only aggregated service health data for the usage page.
func (h *Handler) GetUsageHealth(c *gin.Context) {
	if h == nil || h.usageStats == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "usage statistics unavailable"})
		return
	}

	health, err := h.usageStats.HealthContext(c.Request.Context(), usage.HealthOptions{
		Now: time.Now().UTC(),
	})
	if err != nil {
		if errors.Is(err, usage.ErrHealthUnsupported) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "usage/health is only available when usage statistics storage way is sqlite",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, health)
}
