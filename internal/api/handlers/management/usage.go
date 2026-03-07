package management

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/usage"
)

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

// GetUsageStatistics returns the persisted request statistics snapshot.
func (h *Handler) GetUsageStatistics(c *gin.Context) {
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
	if h != nil && h.usageStats != nil {
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
	}
	c.JSON(http.StatusOK, usageExportPayload{
		Version:    2,
		ExportedAt: time.Now().UTC(),
		Usage:      snapshot,
		Records:    records,
	})
}

// ImportUsageStatistics merges a previously exported usage payload into SQLite.
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
	if payload.Version != 0 && payload.Version != 1 && payload.Version != 2 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported version"})
		return
	}

	var (
		result   usage.MergeResult
		mergeErr error
	)
	if payload.Version == 2 || len(payload.Records) > 0 {
		result, mergeErr = h.usageStats.MergeRecords(c.Request.Context(), payload.Records)
	} else {
		result, mergeErr = h.usageStats.MergeSnapshotContext(c.Request.Context(), payload.Usage)
	}
	if mergeErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": mergeErr.Error()})
		return
	}
	snapshot, snapshotErr := h.usageStats.SnapshotContext(c.Request.Context())
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
