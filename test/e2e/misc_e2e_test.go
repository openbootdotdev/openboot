//go:build e2e && vm

package e2e

import (
	"testing"

	"github.com/openbootdotdev/openboot/testutil"
	"github.com/stretchr/testify/assert"
)

func TestE2E_FullPreset_DryRun(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VM test in short mode")
	}

	vm := testutil.NewMacHost(t)
	vmInstallHomebrew(t, vm)
	bin := vmCopyDevBinary(t, vm)

	env := map[string]string{
		"PATH":               brewPath,
		"OPENBOOT_GIT_NAME":  "Test User",
		"OPENBOOT_GIT_EMAIL": "test@example.com",
	}
	output, err := vm.RunWithEnv(env, bin+" --preset full --dry-run --silent")
	assert.NoError(t, err, "dry-run with full preset should succeed, output: %s", output)
}
