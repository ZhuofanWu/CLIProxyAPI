package management

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/usage"
)

var usageRangeSinceMap = map[string]time.Duration{
	"7h":  7 * time.Hour,
	"24h": 24 * time.Hour,
	"7d":  7 * 24 * time.Hour,
}

type usageExportPayload struct {
	Version    int                      `json:"version"`
	ExportedAt time.Time                `json:"exported_at"`
	Usage      usage.StatisticsSnapshot `json:"usage"`
	Records    []usage.PersistedRecord  `json:"records,omitempty"`
}

type usageImportPayload struct {
	Version int                      `json:"version"`
	Usage   usage.StatisticsSnapshot `json:"usage"`
	Records []usage.PersistedRecord  `json:"records"`
}

// GetUsageStatistics returns the complete usage snapshot for the active storage backend.
func (h *Handler) GetUsageStatistics(c *gin.Context) {
	h.writeUsageStatisticsSnapshot(c)
}

// GetFullUsageStatistics returns the complete usage snapshot.
//
// Deprecated: use /usage instead. This alias will be removed in a future release.
func (h *Handler) GetFullUsageStatistics(c *gin.Context) {
	c.Header("Deprecation", "true")
	c.Header("Link", `</v0/management/usage>; rel="successor-version"`)
	c.Header("Warning", `299 - "/usage/full is deprecated; use /usage"`)
	h.writeUsageStatisticsSnapshot(c)
}

func (h *Handler) writeUsageStatisticsSnapshot(c *gin.Context) {
	var snapshot usage.StatisticsSnapshot
	if h != nil && h.usageStats != nil {
		var err error
		snapshot, err = h.usageStats.SnapshotContext(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"usage":           snapshot,
		"failed_requests": snapshot.FailureCount,
	})
}

// ExportUsageStatistics returns a complete usage payload for backup/migration.
func (h *Handler) ExportUsageStatistics(c *gin.Context) {
	var snapshot usage.StatisticsSnapshot
	var records []usage.PersistedRecord
	version := 1
	if h != nil && h.usageStats != nil {
		if h.usageStats.StorageWay() == usage.UsageStorageWaySQLite {
			version = 2
			var err error
			snapshot, err = h.usageStats.SnapshotContext(c.Request.Context())
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			records, err = h.usageStats.ExportRecords(c.Request.Context())
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		} else {
			snapshot = h.usageStats.Snapshot()
		}
	}
	c.JSON(http.StatusOK, usageExportPayload{
		Version:    version,
		ExportedAt: time.Now().UTC(),
		Usage:      snapshot,
		Records:    records,
	})
}

// ImportUsageStatistics imports a usage payload using the active storage backend semantics.
func (h *Handler) ImportUsageStatistics(c *gin.Context) {
	if h == nil || h.usageStats == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "usage statistics unavailable"})
		return
	}

	data, err := c.GetRawData()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read request body"})
		return
	}

	var payload usageImportPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}

	var (
		result   usage.MergeResult
		mergeErr error
	)
	if h.usageStats.StorageWay() == usage.UsageStorageWaySQLite {
		if payload.Version != 0 && payload.Version != 1 && payload.Version != 2 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported version"})
			return
		}
		if payload.Version == 2 || len(payload.Records) > 0 {
			result, mergeErr = h.usageStats.MergeRecords(c.Request.Context(), payload.Records)
		} else {
			result, mergeErr = h.usageStats.MergeSnapshotContext(c.Request.Context(), payload.Usage)
		}
	} else {
		if payload.Version != 0 && payload.Version != 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported version"})
			return
		}
		result = h.usageStats.MergeSnapshot(payload.Usage)
		mergeErr = nil
	}
	if mergeErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": mergeErr.Error()})
		return
	}

	var snapshotErr error
	var snapshot usage.StatisticsSnapshot
	if h.usageStats.StorageWay() == usage.UsageStorageWaySQLite {
		snapshot, snapshotErr = h.usageStats.SnapshotContext(c.Request.Context())
	} else {
		snapshot = h.usageStats.Snapshot()
	}
	if snapshotErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": snapshotErr.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"added":           result.Added,
		"skipped":         result.Skipped,
		"total_requests":  snapshot.TotalRequests,
		"failed_requests": snapshot.FailureCount,
	})
}

func buildUsageSnapshotOptions(rawRange string) (usage.SnapshotOptions, error) {
	queryRange := strings.ToLower(strings.TrimSpace(rawRange))
	options := usage.SnapshotOptions{}
	if queryRange == "" || queryRange == "all" {
		return options, nil
	}
	duration, ok := usageRangeSinceMap[queryRange]
	if !ok {
		return usage.SnapshotOptions{}, fmt.Errorf("invalid usage range: %s", rawRange)
	}
	options.Since = time.Now().UTC().Add(-duration)
	return options, nil
}
