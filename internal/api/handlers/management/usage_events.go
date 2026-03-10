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

const defaultUsageEventsPage = 1
const defaultUsageEventsPageSize = 100

// GetUsageEvents returns sqlite-only paginated request event detail rows for the usage page.
func (h *Handler) GetUsageEvents(c *gin.Context) {
	if h == nil || h.usageStats == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "usage statistics unavailable"})
		return
	}

	snapshotOptions, err := buildUsageSnapshotOptions(c.Query("range"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	page, err := parsePositiveIntQuery(c.Query("page"), defaultUsageEventsPage)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid page"})
		return
	}
	pageSize, err := parsePositiveIntQuery(c.Query("page_size"), defaultUsageEventsPageSize)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid page_size"})
		return
	}

	var successFilter *bool
	successRaw := strings.TrimSpace(c.Query("success"))
	if successRaw != "" {
		parsed, parseErr := strconv.ParseBool(successRaw)
		if parseErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid success"})
			return
		}
		successFilter = &parsed
	}

	events, err := h.usageStats.EventsContext(c.Request.Context(), usage.UsageEventsOptions{
		Since:     snapshotOptions.Since,
		Now:       time.Now().UTC(),
		ModelName: strings.TrimSpace(c.Query("model")),
		Source:    strings.TrimSpace(c.Query("source")),
		AuthIndex: strings.TrimSpace(c.Query("auth_index")),
		Success:   successFilter,
		Page:      page,
		PageSize:  pageSize,
	})
	if err != nil {
		if errors.Is(err, usage.ErrEventsUnsupported) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "usage/events is only available when usage statistics storage way is sqlite",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, events)
}

func parsePositiveIntQuery(raw string, fallback int) (int, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(trimmed)
	if err != nil || value < 1 {
		return 0, errors.New("invalid positive integer")
	}
	return value, nil
}
