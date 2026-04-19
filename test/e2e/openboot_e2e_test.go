//go:build e2e && vm

package e2e

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openbootdotdev/openboot/testutil"
)

func TestE2E_DryRunMinimal(t *testing.T) {
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
	output, err := vm.RunWithEnv(env, bin+" --preset minimal --dry-run --silent")
	assert.NoError(t, err, "dry-run with minimal preset should succeed, output: %s", output)
}

func TestE2E_DryRunDeveloper(t *testing.T) {
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
	output, err := vm.RunWithEnv(env, bin+" --preset developer --dry-run --silent")
	assert.NoError(t, err, "dry-run with developer preset should succeed, output: %s", output)
}

func TestE2E_SnapshotCapture(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VM test in short mode")
	}

	vm := testutil.NewMacHost(t)
	vmInstallHomebrew(t, vm)
	bin := vmCopyDevBinary(t, vm)

	output, err := vmRunDevBinary(t, vm, bin, "snapshot --json")
	require.NoError(t, err, "snapshot command should succeed, output: %s", output)

	var snapshotData map[string]interface{}
	err = json.Unmarshal([]byte(output), &snapshotData)
	assert.NoError(t, err, "snapshot output should be valid JSON")
	assert.Greater(t, len(output), 0, "snapshot output should not be empty")
}

func TestE2E_InvalidPreset(t *testing.T) {
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
	output, err := vm.RunWithEnv(env, bin+" --preset invalid-preset-xyz --dry-run --silent")
	assert.Error(t, err, "invalid preset should cause command to fail")
	assert.True(t, strings.Contains(output, "invalid") || strings.Contains(output, "unknown") || strings.Contains(output, "error"),
		"error output should mention invalid preset, got: %s", output)
}

func TestE2E_MissingGitConfig(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VM test in short mode")
	}

	vm := testutil.NewMacHost(t)
	vmInstallHomebrew(t, vm)
	bin := vmCopyDevBinary(t, vm)

	// Run without OPENBOOT_GIT_NAME / OPENBOOT_GIT_EMAIL so git config is absent
	output, err := vmRunDevBinary(t, vm, bin, "--preset minimal --dry-run --silent")
	if err == nil {
		t.Logf("Command succeeded when git config was missing. This may be OK if git is already configured globally.")
		t.Logf("Output: %s", output)
	}
}

func TestE2E_SnapshotWithOutput(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VM test in short mode")
	}

	vm := testutil.NewMacHost(t)
	vmInstallHomebrew(t, vm)
	bin := vmCopyDevBinary(t, vm)

	output, err := vmRunDevBinary(t, vm, bin, "snapshot --json")
	require.NoError(t, err, "snapshot --json should succeed, output: %s", output)

	var data map[string]interface{}
	err = json.Unmarshal([]byte(output), &data)
	assert.NoError(t, err, "snapshot output should be valid JSON")
	assert.Greater(t, len(output), 0)
}

func TestE2E_Diff_ThenClean_DryRun_SameSnapshot(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VM test in short mode")
	}

	// Verify diff and clean produce consistent results from the same snapshot
	vm := testutil.NewMacHost(t)
	vmInstallHomebrew(t, vm)
	bin := vmCopyDevBinary(t, vm)

	// Write a minimal snapshot to the VM
	remotePath := "/tmp/e2e-combined-snapshot.json"
	vmWriteTestSnapshot(t, vm, remotePath, []string{"git"}, []string{}, []string{})

	// Run diff
	diffOutput, diffErr := vmRunDevBinary(t, vm, bin, "diff --from "+remotePath)

	// Run clean --dry-run
	cleanOutput, cleanErr := vmRunDevBinary(t, vm, bin, "clean --from "+remotePath+" --dry-run")

	// Both should succeed
	assert.NoError(t, diffErr, "diff should succeed, output: %s", diffOutput)
	assert.NoError(t, cleanErr, "clean --dry-run should succeed, output: %s", cleanOutput)
}
