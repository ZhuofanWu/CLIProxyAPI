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

// GetUsageTokenBreakdown returns sqlite-only token breakdown buckets for the usage page.
func (h *Handler) GetUsageTokenBreakdown(c *gin.Context) {
	if h == nil || h.usageStats == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "usage statistics unavailable"})
		return
	}

	options, err := buildTokenBreakdownOptions(
		c.Query("granularity"),
		c.Query("range"),
		c.Query("offset"),
	)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	snapshot, err := h.usageStats.TokenBreakdownContext(c.Request.Context(), options)
	if err != nil {
		if errors.Is(err, usage.ErrTokenBreakdownUnsupported) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "usage/token-breakdown is only available when usage statistics storage way is sqlite",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, snapshot)
}

func buildTokenBreakdownOptions(
	rawGranularity string,
	rawRange string,
	rawOffset string,
) (usage.TokenBreakdownOptions, error) {
	granularity := strings.ToLower(strings.TrimSpace(rawGranularity))
	if granularity == "" {
		granularity = "hour"
	}
	switch granularity {
	case "hour", "day":
	default:
		return usage.TokenBreakdownOptions{}, errors.New("invalid token breakdown granularity")
	}

	queryRange := strings.ToLower(strings.TrimSpace(rawRange))
	if queryRange == "" {
		queryRange = "all"
	}
	switch queryRange {
	case "7h", "24h", "7d", "all":
	default:
		return usage.TokenBreakdownOptions{}, errors.New("invalid token breakdown range")
	}

	offset := 0
	if strings.TrimSpace(rawOffset) != "" {
		parsedOffset, err := strconv.Atoi(strings.TrimSpace(rawOffset))
		if err != nil {
			return usage.TokenBreakdownOptions{}, errors.New("invalid token breakdown offset")
		}
		if parsedOffset < 0 {
			return usage.TokenBreakdownOptions{}, errors.New("invalid token breakdown offset")
		}
		offset = parsedOffset
	}

	return usage.TokenBreakdownOptions{
		Granularity: granularity,
		Range:       queryRange,
		Offset:      offset,
		Now:         time.Now().UTC(),
	}, nil
}
