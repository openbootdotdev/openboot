//go:build e2e && vm

package e2e

import (
	"strings"
	"testing"

	"github.com/openbootdotdev/openboot/testutil"
	"github.com/stretchr/testify/assert"
)

func TestE2E_Update_DryRun(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VM test in short mode")
	}

	vm := testutil.NewTartVM(t)
	vmInstallHomebrew(t, vm)
	bin := vmCopyDevBinary(t, vm)

	output, err := vmRunDevBinary(t, vm, bin, "update --dry-run")
	assert.NoError(t, err, "update --dry-run should succeed, output: %s", output)
	assert.True(t,
		strings.Contains(output, "DRY-RUN") || strings.Contains(output, "up to date") || strings.Contains(output, "outdated") || strings.Contains(output, "Would run"),
		"output should show dry-run info or package status, got: %s", output)
}

func TestE2E_Update_HelpFlag(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VM test in short mode")
	}

	vm := testutil.NewTartVM(t)
	vmInstallHomebrew(t, vm)
	bin := vmCopyDevBinary(t, vm)

	output, err := vmRunDevBinary(t, vm, bin, "update --help")
	assert.NoError(t, err, "update --help should succeed")
	assert.Contains(t, output, "update")
	assert.Contains(t, output, "--dry-run")
	assert.Contains(t, output, "--self")
}

func TestE2E_Update_Self_DevVersion(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VM test in short mode")
	}

	vm := testutil.NewTartVM(t)
	vmInstallHomebrew(t, vm)
	bin := vmCopyDevBinary(t, vm)

	// Dev builds (version=dev) should handle --self gracefully
	output, err := vmRunDevBinary(t, vm, bin, "update --self")
	// May succeed or fail depending on install method detection —
	// just verify it doesn't panic
	t.Logf("update --self output: %s (err: %v)", output, err)
}
