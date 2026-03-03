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

func TestE2E_Clean_DryRun_FromFile(t *testing.T) {
	binary := testutil.BuildTestBinary(t)
	tmpDir := t.TempDir()

	// Create a snapshot with a small set of packages — system will have "extra"
	snapshotPath := writeTestSnapshot(t, tmpDir, []string{"git"}, []string{}, []string{})

	// Capture installed packages before
	beforePkgs, err := testutil.GetInstalledBrewPackages()
	if err != nil {
		t.Skipf("cannot list brew packages: %v", err)
	}

	cmd := exec.Command(binary, "clean", "--from", snapshotPath, "--dry-run")
	output, err := cmd.CombinedOutput()
	outStr := string(output)

	assert.NoError(t, err, "clean --dry-run should succeed, output: %s", outStr)
	assert.True(t,
		strings.Contains(outStr, "DRY-RUN") || strings.Contains(outStr, "dry run") || strings.Contains(outStr, "Dry run"),
		"output should mention dry-run mode, got: %s", outStr)

	// Verify nothing was actually removed
	afterPkgs, err := testutil.GetInstalledBrewPackages()
	if err != nil {
		t.Skipf("cannot list brew packages: %v", err)
	}
	assert.Equal(t, len(beforePkgs), len(afterPkgs),
		"dry-run should not remove any packages (before=%d, after=%d)", len(beforePkgs), len(afterPkgs))
}

func TestE2E_Clean_MissingFile(t *testing.T) {
	binary := testutil.BuildTestBinary(t)

	cmd := exec.Command(binary, "clean", "--from", "/tmp/nonexistent-snapshot-99999.json")
	output, err := cmd.CombinedOutput()
	outStr := string(output)

	assert.Error(t, err, "clean with missing file should fail")
	assert.True(t,
		strings.Contains(outStr, "not found") || strings.Contains(outStr, "no such file") || strings.Contains(outStr, "snapshot"),
		"error should mention file issue, got: %s", outStr)
}

func TestE2E_Clean_NoLocalSnapshot(t *testing.T) {
	binary := testutil.BuildTestBinary(t)

	// Run clean without flags — tries local snapshot
	cmd := exec.Command(binary, "clean")
	// Set HOME to a temp dir so no local snapshot exists
	tmpHome := t.TempDir()
	cmd.Env = append(os.Environ(), "HOME="+tmpHome)

	output, err := cmd.CombinedOutput()
	outStr := string(output)

	if err != nil {
		assert.True(t,
			strings.Contains(outStr, "snapshot") || strings.Contains(outStr, "--from") || strings.Contains(outStr, "--user"),
			"error should guide user, got: %s", outStr)
	}
}

func TestE2E_Clean_HelpFlag(t *testing.T) {
	binary := testutil.BuildTestBinary(t)

	cmd := exec.Command(binary, "clean", "--help")
	output, err := cmd.CombinedOutput()
	outStr := string(output)

	assert.NoError(t, err, "clean --help should succeed")
	assert.Contains(t, outStr, "clean")
	assert.Contains(t, outStr, "--from")
	assert.Contains(t, outStr, "--user")
	assert.Contains(t, outStr, "--dry-run")
}
