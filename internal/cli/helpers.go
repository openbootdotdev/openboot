package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/openbootdotdev/openboot/internal/config"
	syncpkg "github.com/openbootdotdev/openboot/internal/sync"
	"github.com/openbootdotdev/openboot/internal/ui"
)

const apiRequestTimeout = 30 * time.Second

// parseConflictError interprets a 409 response body from the openboot API.
// Returns a user-friendly error if the body can be decoded, or a raw conflict error.
func parseConflictError(body []byte) error {
	var errResp struct {
		Message string `json:"message"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(body, &errResp); err == nil {
		msg := errResp.Message
		if msg == "" {
			msg = errResp.Error
		}
		if msg != "" && strings.Contains(strings.ToLower(msg), "maximum") {
			return errors.New("config limit reached (max 20): delete an existing config with 'openboot delete <slug>' first")
		}
		if msg != "" {
			return errors.New(msg)
		}
	}
	return fmt.Errorf("conflict: %s", string(body))
}

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
	if err := syncpkg.SaveSource(source, false); err != nil {
		ui.Warn(fmt.Sprintf("Failed to save sync source: %v", err))
	}
}
