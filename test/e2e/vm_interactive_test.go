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

	// Plain `curl … | bash` IS the hazard, so it needs no simulating: bash's
	// stdin is the pipe carrying the script, which means there is no keyboard
	// for a prompt to read from and no way to hand it one.
	//
	// Don't try to make that explicit by redirecting — `curl … | bash < /dev/null`
	// under bash replaces the script itself, so nothing runs at all and every
	// assertion sees empty output. (Under zsh the same line does run the script,
	// which is a good way to convince yourself it works before CI proves it
	// doesn't.)
	//
	// `-s -- --help` passes --help through to the `openboot install` the script
	// exec's into at the end, so these assertions exercise the installer without
	// kicking off a real install.
	installOverExisting := fmt.Sprintf(
		"export NONINTERACTIVE=1 PATH=%q && curl -fsSL https://openboot.dev/install.sh | bash -s -- --help",
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
