package management

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/usage"
)

// GetUsageCredentials returns sqlite-only credential usage aggregates.
func (h *Handler) GetUsageCredentials(c *gin.Context) {
	if h == nil || h.usageStats == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "usage statistics unavailable"})
		return
	}

	snapshotOptions, err := buildUsageSnapshotOptions(c.Query("range"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	percentDataRaw := strings.TrimSpace(c.DefaultQuery("percentdata", "false"))
	includePercentData, err := strconv.ParseBool(percentDataRaw)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid percentdata"})
		return
	}

	rangeValue := strings.ToLower(strings.TrimSpace(c.Query("range")))
	if rangeValue == "" {
		rangeValue = "all"
	}

	credentials, err := h.usageStats.CredentialsContext(c.Request.Context(), usage.CredentialsOptions{
		Since:              snapshotOptions.Since,
		Now:                time.Now().UTC(),
		Range:              rangeValue,
		IncludePercentData: includePercentData,
	})
	if err != nil {
		if errors.Is(err, usage.ErrCredentialsUnsupported) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "usage/credentials is only available when usage statistics storage way is sqlite",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, credentials)
}
