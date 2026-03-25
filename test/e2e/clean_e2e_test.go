//go:build e2e && vm

package e2e

import (
	"strings"
	"testing"

	"github.com/openbootdotdev/openboot/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestE2E_Clean_DryRun_FromFile(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VM test in short mode")
	}

	vm := testutil.NewTartVM(t)
	vmInstallHomebrew(t, vm)
	bin := vmCopyDevBinary(t, vm)

	// Create a snapshot with a small set of packages — system will have "extra"
	snapshotPath := "/tmp/clean-test-snapshot.json"
	vmWriteTestSnapshot(t, vm, snapshotPath, []string{"git"}, []string{}, []string{})

	// Capture installed packages before
	beforePkgs := vmBrewList(t, vm)

	output, err := vmRunDevBinary(t, vm, bin, "clean --from "+snapshotPath+" --dry-run")
	assert.NoError(t, err, "clean --dry-run should succeed, output: %s", output)
	assert.True(t,
		strings.Contains(output, "DRY-RUN") || strings.Contains(output, "dry run") || strings.Contains(output, "Dry run"),
		"output should mention dry-run mode, got: %s", output)

	// Verify nothing was actually removed
	afterPkgs := vmBrewList(t, vm)
	assert.Equal(t, len(beforePkgs), len(afterPkgs),
		"dry-run should not remove any packages (before=%d, after=%d)", len(beforePkgs), len(afterPkgs))
}

func TestE2E_Clean_MissingFile(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VM test in short mode")
	}

	vm := testutil.NewTartVM(t)
	vmInstallHomebrew(t, vm)
	bin := vmCopyDevBinary(t, vm)

	output, err := vmRunDevBinary(t, vm, bin, "clean --from /tmp/nonexistent-snapshot-99999.json")
	assert.Error(t, err, "clean with missing file should fail")
	assert.True(t,
		strings.Contains(output, "not found") || strings.Contains(output, "no such file") || strings.Contains(output, "snapshot"),
		"error should mention file issue, got: %s", output)
}

func TestE2E_Clean_NoLocalSnapshot(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VM test in short mode")
	}

	vm := testutil.NewTartVM(t)
	vmInstallHomebrew(t, vm)
	bin := vmCopyDevBinary(t, vm)

	// Run clean without flags — tries local snapshot which won't exist in fresh VM
	// Use a temp HOME dir in the VM with no snapshot
	_, err := vm.Run("mkdir -p /tmp/clean-no-snapshot-home")
	require.NoError(t, err)

	output, err := vm.RunWithEnv(
		map[string]string{"PATH": brewPath, "HOME": "/tmp/clean-no-snapshot-home"},
		bin+" clean",
	)
	if err != nil {
		assert.True(t,
			strings.Contains(output, "snapshot") || strings.Contains(output, "--from") || strings.Contains(output, "--user"),
			"error should guide user, got: %s", output)
	}
}

func TestE2E_Clean_HelpFlag(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VM test in short mode")
	}

	vm := testutil.NewTartVM(t)
	vmInstallHomebrew(t, vm)
	bin := vmCopyDevBinary(t, vm)

	output, err := vmRunDevBinary(t, vm, bin, "clean --help")
	assert.NoError(t, err, "clean --help should succeed")
	assert.Contains(t, output, "clean")
	assert.Contains(t, output, "--from")
	assert.Contains(t, output, "--user")
	assert.Contains(t, output, "--dry-run")
}
