package usage

import (
	"context"
	"strings"
	"sync/atomic"

	coreusage "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/usage"
	log "github.com/sirupsen/logrus"
)

var statisticsEnabled atomic.Bool

func init() {
	statisticsEnabled.Store(true)
	coreusage.RegisterPlugin(NewLoggerPlugin())
}

// LoggerPlugin collects request statistics into the shared persistent store.
// It implements coreusage.Plugin to receive usage records emitted by the runtime.
type LoggerPlugin struct {
	stats *RequestStatistics
}

// NewLoggerPlugin constructs a logger plugin wired to the shared statistics store.
func NewLoggerPlugin() *LoggerPlugin { return &LoggerPlugin{stats: defaultRequestStatistics} }

// HandleUsage implements coreusage.Plugin.
func (p *LoggerPlugin) HandleUsage(ctx context.Context, record coreusage.Record) {
	if !statisticsEnabled.Load() {
		return
	}
	if p == nil || p.stats == nil {
		return
	}
	p.stats.Record(ctx, record)
}

// SetStatisticsEnabled toggles whether usage records are persisted.
func SetStatisticsEnabled(enabled bool) { statisticsEnabled.Store(enabled) }

// StatisticsEnabled reports the current recording state.
func StatisticsEnabled() bool { return statisticsEnabled.Load() }

var defaultRequestStatistics = NewRequestStatistics()

// GetRequestStatistics returns the shared statistics store.
func GetRequestStatistics() *RequestStatistics { return defaultRequestStatistics }

// ConfigurePersistentStore configures the shared store to persist under authDir.
func ConfigurePersistentStore(authDir string) error {
	if defaultRequestStatistics == nil {
		return nil
	}
	if err := defaultRequestStatistics.Configure(authDir); err != nil {
		return err
	}
	if path := strings.TrimSpace(defaultRequestStatistics.DatabasePath()); path != "" {
		log.Infof("usage statistics database ready: %s", path)
	}
	return nil
}

// ClosePersistentStore closes the shared persistent store.
func ClosePersistentStore() error {
	if defaultRequestStatistics == nil {
		return nil
	}
	return defaultRequestStatistics.Close()
}
