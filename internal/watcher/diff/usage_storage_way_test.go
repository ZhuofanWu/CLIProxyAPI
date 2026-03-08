package diff

import (
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
)

func TestBuildConfigChangeDetails_UsageStorageWay(t *testing.T) {
	oldCfg := &config.Config{UsageStatisticsStorageWay: config.UsageStatisticsStorageWayMemory}
	newCfg := &config.Config{UsageStatisticsStorageWay: config.UsageStatisticsStorageWaySQLite}

	changes := BuildConfigChangeDetails(oldCfg, newCfg)
	expectContains(t, changes, "usage_statistics_storage_way: memory -> sqlite")
}
