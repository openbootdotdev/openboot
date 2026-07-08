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

// TestVM_WizardTUI_RealInstall drives the full-screen install wizard through a
// REAL install on the CI macOS runner: boot → hand-pick → filter → toggle →
// git identity → live streaming install → DONE screen, then asserts the real
// system state (brew package present, git identity written).
//
// Gap filled: the wizard's streaming path (progress sink + real brew) is the
// one seam no other tier reaches — unit tests fake the engine, the L3
// choreography test (install_wizard_e2e_test.go) stops right before the final
// confirm, and the other L4 tests install via the silent (non-TUI) path. The
// key sequence here mirrors TestE2E_InstallWizard_FullChoreography exactly;
// keep the two in sync.
func TestVM_WizardTUI_RealInstall(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VM wizard test in short mode")
	}

	vm := testutil.NewMacHost(t)
	vmInstallHomebrew(t, vm)

	// expect(1) drives the TUI; install it if the runner lacks it.
	if _, err := vm.Run(fmt.Sprintf("export PATH=%q && command -v expect", brewPath)); err != nil {
		out, installErr := vm.Run(fmt.Sprintf("export PATH=%q && brew install expect", brewPath))
		t.Logf("install expect: %s", out)
		require.NoError(t, installErr, "should install expect for interactive tests")
	}

	binary := testutil.BuildTestBinary(t)
	home := t.TempDir() // fresh HOME → wizard branch; shell/dotfiles land here
	gitCfg := home + "/gitconfig-test"

	// Deterministic target: make sure the formula is absent before the run.
	const formula = "stow"
	vm.Run(fmt.Sprintf("export PATH=%q && brew uninstall --ignore-dependencies %s", brewPath, formula)) //nolint:errcheck // absent is fine

	// Isolated env: dead localhost API → embedded catalog; git config
	// redirected so the identity screen deterministically appears and the
	// written identity is assertable without touching the runner's config.
	cmd := fmt.Sprintf(
		"export HOME=%q GIT_CONFIG_GLOBAL=%q GIT_CONFIG_SYSTEM=/dev/null"+
			" OPENBOOT_DISABLE_AUTOUPDATE=1 OPENBOOT_API_URL=http://localhost:1"+
			" TERM=xterm-256color PATH=%q && stty rows 32 cols 110 && %s install",
		home, gitCfg, brewPath, binary,
	)

	// Every Expect marker lies within a single styled span, so ANSI codes
	// can't split it. "snapshot publish" is the DONE-screen next-step hint —
	// it renders on both clean and soft-error completion, so the expect
	// script can't hang; success is asserted on "dev-ready" below.
	output, err := vm.RunInteractive(cmd, []testutil.ExpectStep{
		{Expect: "Choose a starting point", Send: "c"},
		{Expect: "type to filter", Send: "/" + formula + "\r"},
		{Expect: "1 pkgs", Send: "\r"},
		{Expect: "Set your git identity", Send: "CI Bot\tci@openboot.test\r"},
		{Expect: "snapshot publish", Send: "q"},
	}, 900)
	t.Logf("wizard session:\n%s", output)
	if err != nil {
		t.Logf("expect exited with: %v", err)
	}

	// The DONE screen reported a clean install (soft errors render
	// "Finished with some errors" instead).
	assert.Contains(t, output, "dev-ready", "install should finish clean")

	// Real system state: the package actually installed…
	listOut, listErr := vm.Run(fmt.Sprintf("export PATH=%q && brew list --formula %s", brewPath, formula))
	assert.NoError(t, listErr, "%s should be installed via brew: %s", formula, listOut)

	// …and the captured git identity actually written.
	nameOut, nameErr := vm.Run(fmt.Sprintf("export GIT_CONFIG_GLOBAL=%q GIT_CONFIG_SYSTEM=/dev/null && git config --global user.name", gitCfg))
	assert.NoError(t, nameErr, "git identity should be configured")
	assert.Equal(t, "CI Bot", strings.TrimSpace(nameOut), "git user.name from the TUI capture")

	// System-config steps ran against the isolated HOME.
	_, omzErr := vm.Run(fmt.Sprintf("test -d %q", home+"/.oh-my-zsh"))
	assert.NoError(t, omzErr, "oh-my-zsh should be installed into the wizard's HOME")
}
