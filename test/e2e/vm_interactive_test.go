//go:build e2e && vm

package e2e

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openbootdotdev/openboot/testutil"
)

// TestVM_Interactive_InstallScript covers install.sh when openboot is already
// installed — the path every returning user takes.
//
// This file used to drive a "Reinstall? (y/N)" prompt with expect and assert
// that answering "n" kept the existing install. It passed, and it verified
// nothing: under `curl … | bash` the script's stdin is the pipe carrying the
// script, so `read` never saw expect's keystroke. It consumed the script's own
// bytes, failed to match ^[Yy]$, and took the No branch — precisely what the
// test asserted. Bug and assertion agreed, so the suite stayed green while
// every returning user was silently pinned to their first-installed version.
//
// The prompt is gone: running the installer means you want the current
// release. These tests pin that contract instead.
func TestVM_Interactive_InstallScript(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VM interactive test in short mode")
	}

	vm := testutil.NewMacHost(t)
	vmInstallViaBrew(t, vm) // taps openbootdotdev/openboot and installs openboot

	// `< /dev/null` reproduces the real hazard: under curl|bash there is no
	// keyboard on stdin. A prompt here must not block, and must not silently
	// answer itself from the script's own bytes.
	installOverExisting := fmt.Sprintf(
		"export NONINTERACTIVE=1 PATH=%q && curl -fsSL https://openboot.dev/install.sh | bash < /dev/null",
		brewPath,
	)

	t.Run("already_installed_updates_without_prompting", func(t *testing.T) {
		output, err := vm.Run(installOverExisting)
		t.Logf("install-over-existing:\n%s", output)
		if err != nil {
			t.Logf("exited with: %v", err) // it exec's into `openboot install`, which wants a tty
		}

		assert.NotContains(t, output, "Reinstall?",
			"must not ask a question it cannot receive the answer to under curl|bash")
		assert.NotContains(t, output, "Using existing installation",
			"an existing install must be updated, not silently kept")
		assert.Contains(t, output, "updating",
			"the already-installed path reports that it is updating, got: %s", output)
	})

	// The failure this path guards against is a successful-looking install that
	// leaves yesterday's binary behind, so the version has to be stated where
	// the user can see it.
	t.Run("reports_the_version_it_installed", func(t *testing.T) {
		output, _ := vm.Run(installOverExisting)

		version, err := vm.Run(fmt.Sprintf("export PATH=%q && openboot version", brewPath))
		require.NoError(t, err, "openboot must be runnable after the installer")
		version = strings.TrimSpace(version)
		require.NotEmpty(t, version)

		assert.Contains(t, output, version,
			"installer must print the version it left behind (%q), got: %s", version, output)
	})
}
