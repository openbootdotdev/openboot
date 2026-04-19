//go:build e2e && vm

package e2e

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/openbootdotdev/openboot/testutil"
)

// TestVM_Interactive_InstallScript tests install.sh interactive prompts.
func TestVM_Interactive_InstallScript(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VM interactive test in short mode")
	}

	vm := testutil.NewMacHost(t)
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
