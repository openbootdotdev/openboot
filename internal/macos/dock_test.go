package macos

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSetDockApps_DryRunDeleteThenAddThenKillall(t *testing.T) {
	out := captureStdout(t, func() {
		err := SetDockApps([]string{
			"/Applications/Chrome.app",
			"/Applications/Zed.app",
		}, true /* dryRun */)
		assert.NoError(t, err)
	})
	deleteIdx := strings.Index(out, "defaults delete com.apple.dock persistent-apps")
	chromeIdx := strings.Index(out, "Chrome.app")
	zedIdx := strings.Index(out, "Zed.app")
	killIdx := strings.Index(out, "killall Dock")
	assert.NotEqual(t, -1, deleteIdx, "missing delete step")
	assert.NotEqual(t, -1, chromeIdx, "missing chrome add")
	assert.NotEqual(t, -1, zedIdx, "missing zed add")
	assert.NotEqual(t, -1, killIdx, "missing killall Dock")
	assert.Less(t, deleteIdx, chromeIdx, "delete must come before adds")
	assert.Less(t, chromeIdx, zedIdx, "chrome (first in list) must come before zed")
	assert.Less(t, zedIdx, killIdx, "killall must come after adds")
}

func TestSetDockApps_DryRunEmptyClearsAndExits(t *testing.T) {
	out := captureStdout(t, func() {
		err := SetDockApps([]string{}, true)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "defaults delete com.apple.dock persistent-apps")
	assert.NotContains(t, out, "-array-add")
}

func TestSetDockApps_DryRunMissingAppPathSkippedWithWarn(t *testing.T) {
	// Create a real .app directory so os.Stat succeeds for the "present" app.
	dir := t.TempDir()
	realApp := dir + "/Real.app"
	if err := os.MkdirAll(realApp, 0755); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		err := SetDockApps([]string{
			"/Applications/DefinitelyDoesNotExist123.app",
			realApp,
		}, true)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "DefinitelyDoesNotExist123.app")
	assert.Contains(t, out, "Real.app")
	addLines := 0
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "-array-add") {
			addLines++
		}
	}
	assert.Equal(t, 1, addLines, "only Real.app should be added")
}
