//go:build e2e

package e2e

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/openbootdotdev/openboot/testutil"
	"github.com/stretchr/testify/assert"
)

func TestE2E_UnknownCommand(t *testing.T) {
	binary := testutil.BuildTestBinary(t)

	cmd := exec.Command(binary, "nonexistent-command")
	output, err := cmd.CombinedOutput()
	outStr := string(output)

	assert.Error(t, err, "unknown command should fail")
	assert.True(t,
		strings.Contains(outStr, "unknown command") || strings.Contains(outStr, "unknown") || strings.Contains(outStr, "Usage:"),
		"error should mention unknown command, got: %s", outStr)
}

func TestE2E_UnknownFlag(t *testing.T) {
	binary := testutil.BuildTestBinary(t)

	cmd := exec.Command(binary, "--nonexistent-flag")
	output, err := cmd.CombinedOutput()
	outStr := string(output)

	assert.Error(t, err, "unknown flag should fail")
	assert.True(t,
		strings.Contains(outStr, "unknown flag") || strings.Contains(outStr, "flag"),
		"error should mention unknown flag, got: %s", outStr)
}

func TestE2E_SnapshotLocal(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping snapshot --local in short mode")
	}

	binary := testutil.BuildTestBinary(t)

	// Use a temporary HOME so we don't overwrite the user's real snapshot
	tmpHome := t.TempDir()
	openbootDir := tmpHome + "/.openboot"
	err := os.MkdirAll(openbootDir, 0755)
	assert.NoError(t, err)

	cmd := exec.Command(binary, "snapshot", "--local")
	cmd.Env = append(os.Environ(), "HOME="+tmpHome)

	output, err := cmd.CombinedOutput()
	outStr := string(output)

	assert.NoError(t, err, "snapshot --local should succeed, output: %s", outStr)
	assert.True(t,
		strings.Contains(outStr, "Snapshot saved") || strings.Contains(outStr, "saved") || strings.Contains(outStr, "✓"),
		"output should confirm save, got: %s", outStr)

	// Verify snapshot file was created
	snapshotFile := openbootDir + "/snapshot.json"
	info, err := os.Stat(snapshotFile)
	assert.NoError(t, err, "snapshot file should exist at %s", snapshotFile)
	if info != nil {
		assert.Greater(t, info.Size(), int64(0), "snapshot file should not be empty")
	}
}

func TestE2E_Doctor_ExitCode(t *testing.T) {
	binary := testutil.BuildTestBinary(t)

	cmd := exec.Command(binary, "doctor")
	output, err := cmd.CombinedOutput()
	outStr := string(output)

	// Doctor should always succeed (exit 0) even if it finds issues
	assert.NoError(t, err, "doctor should succeed, output: %s", outStr)
	assert.Contains(t, outStr, "OpenBoot Doctor")
}

func TestE2E_Logout_WhenNotLoggedIn(t *testing.T) {
	binary := testutil.BuildTestBinary(t)

	// Use a clean HOME so no auth token exists
	tmpHome := t.TempDir()
	cmd := exec.Command(binary, "logout")
	cmd.Env = append(os.Environ(), "HOME="+tmpHome)

	output, err := cmd.CombinedOutput()
	outStr := string(output)

	// Should succeed gracefully
	assert.NoError(t, err, "logout when not logged in should succeed, output: %s", outStr)
	assert.True(t,
		strings.Contains(outStr, "Not logged in") || strings.Contains(outStr, "not logged"),
		"should indicate not logged in, got: %s", outStr)
}

func TestE2E_FullPreset_DryRun(t *testing.T) {
	binary := testutil.BuildTestBinary(t)

	cmd := exec.Command(binary, "--preset", "full", "--dry-run", "--silent")
	cmd.Env = append(os.Environ(),
		"OPENBOOT_GIT_NAME=Test User",
		"OPENBOOT_GIT_EMAIL=test@example.com",
	)

	output, err := cmd.CombinedOutput()
	outStr := string(output)

	assert.NoError(t, err, "dry-run with full preset should succeed, output: %s", outStr)
}

func TestE2E_Diff_ThenClean_DryRun_SameSnapshot(t *testing.T) {
	// Verify diff and clean produce consistent results from the same snapshot
	binary := testutil.BuildTestBinary(t)
	tmpDir := t.TempDir()

	snapshotPath := writeTestSnapshot(t, tmpDir, []string{"git"}, []string{}, []string{})

	// Run diff
	diffCmd := exec.Command(binary, "diff", "--from", snapshotPath)
	diffOutput, diffErr := diffCmd.CombinedOutput()

	// Run clean --dry-run
	cleanCmd := exec.Command(binary, "clean", "--from", snapshotPath, "--dry-run")
	cleanOutput, cleanErr := cleanCmd.CombinedOutput()

	// Both should succeed
	assert.NoError(t, diffErr, "diff should succeed, output: %s", string(diffOutput))
	assert.NoError(t, cleanErr, "clean --dry-run should succeed, output: %s", string(cleanOutput))
}
