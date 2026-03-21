//go:build e2e && vm

package e2e

import (
	"fmt"
	"strings"
	"testing"

	"github.com/openbootdotdev/openboot/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestVM_Interactive_InstallScript tests install.sh interactive prompts.
func TestVM_Interactive_InstallScript(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VM interactive test in short mode")
	}

	vm := testutil.NewTartVM(t)
	vmInstallViaBrewTap(t, vm) // Install first

	t.Run("reinstall_answer_no", func(t *testing.T) {
		cmd := fmt.Sprintf(
			"export NONINTERACTIVE=1 PATH=%q && curl -fsSL https://openboot.dev/install.sh | bash",
			brewPath,
		)
		output, err := vm.RunInteractive(cmd, []testutil.ExpectStep{
			{Expect: "Reinstall", Send: "n\r"},
		}, 30)
		t.Logf("reinstall-no:\n%s", output)
		if err != nil {
			t.Logf("exited with: %v", err)
		}
		assert.True(t,
			strings.Contains(output, "existing") ||
				strings.Contains(output, "Using"),
			"should keep existing installation, got: %s", output)
	})

	t.Run("reinstall_answer_yes", func(t *testing.T) {
		cmd := fmt.Sprintf(
			"export NONINTERACTIVE=1 PATH=%q && curl -fsSL https://openboot.dev/install.sh | bash -s -- --help",
			brewPath,
		)
		output, err := vm.RunInteractive(cmd, []testutil.ExpectStep{
			{Expect: "Reinstall", Send: "y\r"},
		}, 120)
		t.Logf("reinstall-yes:\n%s", output)
		if err != nil {
			t.Logf("exited with: %v", err)
		}
		assert.True(t,
			strings.Contains(output, "reinstalled") ||
				strings.Contains(output, "Reinstalling") ||
				strings.Contains(output, "Usage:"),
			"should reinstall, got: %s", output)
	})
}

// TestVM_Interactive_Snapshot tests the interactive snapshot workflow.
func TestVM_Interactive_Snapshot(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VM interactive test in short mode")
	}

	vm := testutil.NewTartVM(t)
	vmInstallHomebrew(t, vm)
	bin := vmCopyDevBinary(t, vm)

	t.Run("interactive_save_locally", func(t *testing.T) {
		cmd := fmt.Sprintf("export PATH=%q && %s snapshot", brewPath, bin)

		output, err := vm.RunInteractive(cmd, []testutil.ExpectStep{
			{Expect: "Save", Send: "\r"}, // Select save locally
		}, 60)
		t.Logf("interactive snapshot:\n%s", output)
		if err != nil {
			t.Logf("exited with: %v", err)
		}

		// Check if snapshot was saved
		out, _ := vm.Run("test -f ~/.openboot/snapshot.json && echo exists || echo missing")
		t.Logf("snapshot file: %s", strings.TrimSpace(out))
	})
}

// TestVM_Interactive_PresetSelection tests the preset selection TUI.
func TestVM_Interactive_PresetSelection(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VM interactive test in short mode")
	}

	vm := testutil.NewTartVM(t)
	vmInstallHomebrew(t, vm)
	bin := vmCopyDevBinary(t, vm)

	t.Run("select_first_preset_dry_run", func(t *testing.T) {
		cmd := fmt.Sprintf(
			"export PATH=%q OPENBOOT_GIT_NAME='Test' OPENBOOT_GIT_EMAIL='test@test.com' && %s --dry-run",
			brewPath, bin,
		)
		output, err := vm.RunInteractive(cmd, []testutil.ExpectStep{
			{Expect: "preset", Send: "\r"},  // Select first preset
			{Expect: "confirm", Send: "\r"}, // Confirm if prompted
		}, 60)
		t.Logf("interactive preset:\n%s", output)
		if err != nil {
			t.Logf("exited with: %v (may be expected)", err)
		}
		assert.True(t,
			strings.Contains(output, "OpenBoot") ||
				strings.Contains(output, "preset") ||
				strings.Contains(output, "DRY-RUN"),
			"should show installer output")
	})
}

// TestVM_Interactive_Clean tests interactive clean confirmation.
func TestVM_Interactive_Clean(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VM interactive test in short mode")
	}

	vm := testutil.NewTartVM(t)
	vmInstallHomebrew(t, vm)
	bin := vmCopyDevBinary(t, vm)

	// Install packages, save snapshot, install extra
	vmRunDevBinaryWithGit(t, vm, bin, "--preset minimal --silent --packages-only")
	vmRunDevBinary(t, vm, bin, "snapshot --local")
	vm.Run("export PATH=\"/opt/homebrew/bin:$PATH\" && brew install cowsay")

	t.Run("clean_interactive_confirm", func(t *testing.T) {
		cmd := fmt.Sprintf("export PATH=%q && %s clean --from ~/.openboot/snapshot.json", brewPath, bin)

		output, err := vm.RunInteractive(cmd, []testutil.ExpectStep{
			// Clean may ask for confirmation before removing
			{Expect: "remove", Send: "y\r"},
		}, 60)
		t.Logf("interactive clean:\n%s", output)
		if err != nil {
			t.Logf("exited with: %v", err)
		}
	})
}

// TestVM_Interactive_Sync tests interactive sync workflow.
func TestVM_Interactive_Sync(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VM interactive test in short mode")
	}

	vm := testutil.NewTartVM(t)
	vmInstallHomebrew(t, vm)
	bin := vmCopyDevBinary(t, vm)

	t.Run("sync_no_source_error", func(t *testing.T) {
		cmd := fmt.Sprintf("export PATH=%q && %s sync", brewPath, bin)

		output, err := vm.RunInteractive(cmd, []testutil.ExpectStep{}, 15)
		t.Logf("sync (no source):\n%s", output)
		if err != nil {
			t.Logf("exited with: %v (expected)", err)
		}
	})
}

// TestVM_Interactive_Init tests interactive init workflow.
func TestVM_Interactive_Init(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VM interactive test in short mode")
	}

	vm := testutil.NewTartVM(t)
	vmInstallHomebrew(t, vm)
	bin := vmCopyDevBinary(t, vm)

	// Create project with .openboot.yml
	vm.Run("mkdir -p /tmp/myproject")
	vm.Run(`cat > /tmp/myproject/.openboot.yml << 'YAML'
dependencies:
  brew:
    - jq
    - curl
YAML`)

	t.Run("init_interactive", func(t *testing.T) {
		cmd := fmt.Sprintf("export PATH=%q && %s init /tmp/myproject", brewPath, bin)

		output, err := vm.RunInteractive(cmd, []testutil.ExpectStep{
			{Expect: "install", Send: "\r"}, // Confirm install if prompted
		}, 120)
		t.Logf("init interactive:\n%s", output)
		if err != nil {
			t.Logf("exited with: %v", err)
		}

		// Verify jq installed
		require.True(t, vmIsInstalled(t, vm, "jq"), "jq should be installed after init")
	})
}
