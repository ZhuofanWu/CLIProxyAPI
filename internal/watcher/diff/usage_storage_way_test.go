package diff

import (
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
)

func TestBuildConfigChangeDetails_UsageStorageWay(t *testing.T) {
	oldCfg := &config.Config{UsageStaticStorageWay: config.UsageStaticStorageWayMemory}
	newCfg := &config.Config{UsageStaticStorageWay: config.UsageStaticStorageWaySQLite}

	changes := BuildConfigChangeDetails(oldCfg, newCfg)
	expectContains(t, changes, "usage_static_storage_way: memory -> sqlite")
}
