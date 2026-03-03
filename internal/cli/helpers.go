package cli

import (
	"fmt"
	"time"

	"github.com/openbootdotdev/openboot/internal/config"
	syncpkg "github.com/openbootdotdev/openboot/internal/sync"
	"github.com/openbootdotdev/openboot/internal/ui"
)

// saveSyncSourceIfRemote persists the remote config reference so that
// `openboot sync` can re-use it later without requiring --source.
func saveSyncSourceIfRemote(c *config.Config) {
	if c.RemoteConfig == nil {
		return
	}
	source := &syncpkg.SyncSource{
		UserSlug:    c.User,
		Username:    c.RemoteConfig.Username,
		Slug:        c.RemoteConfig.Slug,
		InstalledAt: time.Now(),
	}
	if err := syncpkg.SaveSource(source); err != nil {
		ui.Warn(fmt.Sprintf("Failed to save sync source: %v", err))
	}
}
