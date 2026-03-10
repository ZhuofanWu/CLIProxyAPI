package management

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/usage"
)

// GetUsageRequestTrend returns sqlite-only request trend buckets for the usage page.
func (h *Handler) GetUsageRequestTrend(c *gin.Context) {
	h.getUsageMetricTrend(c, usageTrendMetricRequests)
}

// GetUsageTokenTrend returns sqlite-only token trend buckets for the usage page.
func (h *Handler) GetUsageTokenTrend(c *gin.Context) {
	h.getUsageMetricTrend(c, usageTrendMetricTokens)
}

// GetUsageModels returns sqlite-only selectable model names for the usage page trend charts.
func (h *Handler) GetUsageModels(c *gin.Context) {
	if h == nil || h.usageStats == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "usage statistics unavailable"})
		return
	}

	now := time.Now().UTC()
	options, err := buildUsageTrendModelsOptions(c.Query("range"), now)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	snapshot, err := h.usageStats.TrendModelsContext(c.Request.Context(), options)
	if err != nil {
		if errors.Is(err, usage.ErrTrendModelsUnsupported) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "usage/models is only available when usage statistics storage way is sqlite",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, snapshot)
}

const (
	usageTrendMetricRequests = "requests"
	usageTrendMetricTokens   = "tokens"
)

func (h *Handler) getUsageMetricTrend(c *gin.Context, metric string) {
	if h == nil || h.usageStats == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "usage statistics unavailable"})
		return
	}

	now := time.Now().UTC()
	options, err := buildUsageTrendOptions(
		c.Query("granularity"),
		c.Query("range"),
		c.QueryArray("model"),
		now,
	)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	snapshot, err := h.usageStats.TrendContext(c.Request.Context(), options)
	if err != nil {
		if errors.Is(err, usage.ErrTrendUnsupported) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "usage trends are only available when usage statistics storage way is sqlite",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	result := usage.MetricTrendSnapshot{
		Metric:      metric,
		Granularity: snapshot.Granularity,
		Range:       snapshot.Range,
		Labels:      append([]string(nil), snapshot.Labels...),
		Series:      make([]usage.MetricTrendSeries, 0, len(snapshot.Series)),
	}
	for _, series := range snapshot.Series {
		item := usage.MetricTrendSeries{
			ModelName: series.ModelName,
			IsAll:     series.IsAll,
		}
		if metric == usageTrendMetricTokens {
			item.Values = append([]int64(nil), series.Tokens...)
		} else {
			item.Values = append([]int64(nil), series.Requests...)
		}
		result.Series = append(result.Series, item)
	}

	c.JSON(http.StatusOK, result)
}

func buildUsageTrendOptions(
	rawGranularity string,
	rawRange string,
	rawModels []string,
	now time.Time,
) (usage.TrendOptions, error) {
	granularity := strings.ToLower(strings.TrimSpace(rawGranularity))
	if granularity == "" {
		granularity = "day"
	}
	switch granularity {
	case "hour", "day":
	default:
		return usage.TrendOptions{}, errors.New("invalid usage trend granularity")
	}

	queryRange := strings.ToLower(strings.TrimSpace(rawRange))
	if queryRange == "" {
		queryRange = "all"
	}
	switch queryRange {
	case "7h", "24h", "7d", "all":
	default:
		return usage.TrendOptions{}, errors.New("invalid usage trend range")
	}

	models := make([]string, 0, len(rawModels))
	for _, rawModel := range rawModels {
		modelName := strings.TrimSpace(rawModel)
		if modelName == "" {
			continue
		}
		models = append(models, modelName)
	}

	return usage.TrendOptions{
		Granularity: granularity,
		Range:       queryRange,
		Now:         now,
		Models:      models,
	}, nil
}

func buildUsageTrendModelsOptions(rawRange string, now time.Time) (usage.TrendModelsOptions, error) {
	queryRange := strings.ToLower(strings.TrimSpace(rawRange))
	options := usage.TrendModelsOptions{
		Now:   now,
		Range: "all",
	}
	if queryRange == "" || queryRange == "all" {
		return options, nil
	}

	duration, ok := usageRangeSinceMap[queryRange]
	if !ok {
		return usage.TrendModelsOptions{}, errors.New("invalid usage range")
	}

	options.Range = queryRange
	options.Since = now.Add(-duration)
	return options, nil
}
