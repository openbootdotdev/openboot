//go:build e2e && destructive && smoke

package e2e

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/openbootdotdev/openboot/internal/snapshot"
	"github.com/openbootdotdev/openboot/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func captureSnapshot(t *testing.T, binary string) snapshot.Snapshot {
	t.Helper()

	cmd := exec.Command(binary, "snapshot", "--json")
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	require.NoError(t, err, "snapshot capture failed, stderr: %s", stderr.String())

	var snap snapshot.Snapshot
	err = json.Unmarshal([]byte(stdout.String()), &snap)
	require.NoError(t, err, "snapshot JSON parse failed")

	return snap
}

func TestSmoke_InstallAndVerifySnapshot(t *testing.T) {
	binary := testutil.BuildTestBinary(t)
	testPkg := "cowsay"

	// Given: cowsay is not installed
	testutil.EnsurePackageNotInstalled(t, testPkg)

	// Capture before snapshot
	before := captureSnapshot(t, binary)
	beforeFormulae := make(map[string]bool, len(before.Packages.Formulae))
	for _, f := range before.Packages.Formulae {
		beforeFormulae[f] = true
	}
	require.False(t, beforeFormulae[testPkg], "cowsay should not be in before snapshot")

	// When: install cowsay via brew directly (simulates what openboot does)
	installCmd := exec.Command("brew", "install", testPkg)
	require.NoError(t, installCmd.Run(), "brew install cowsay should succeed")
	t.Cleanup(func() { testutil.UninstallPackage(t, testPkg) })

	// Capture after snapshot
	after := captureSnapshot(t, binary)
	afterFormulae := make(map[string]bool, len(after.Packages.Formulae))
	for _, f := range after.Packages.Formulae {
		afterFormulae[f] = true
	}

	// Then: cowsay should appear in after but not before
	assert.True(t, afterFormulae[testPkg], "cowsay should be in after snapshot")

	// Verify only expected change: cowsay was added
	added := []string{}
	for _, f := range after.Packages.Formulae {
		if !beforeFormulae[f] {
			added = append(added, f)
		}
	}
	assert.Contains(t, added, testPkg, "cowsay should be in added packages")
}

func TestSmoke_DryRunNoSideEffects(t *testing.T) {
	binary := testutil.BuildTestBinary(t)

	// Capture before snapshot
	before := captureSnapshot(t, binary)

	// When: run with --dry-run --preset full
	cmd := exec.Command(binary, "install", "--preset", "full", "--dry-run", "--silent")
	cmd.Env = append(os.Environ(),
		"OPENBOOT_GIT_NAME=Smoke Test",
		"OPENBOOT_GIT_EMAIL=smoke@test.local",
	)
	output, err := cmd.CombinedOutput()
	t.Logf("Dry-run output: %s", string(output))
	assert.NoError(t, err, "dry-run should succeed")

	// Capture after snapshot
	after := captureSnapshot(t, binary)

	// Then: formulae lists should be identical (use ElementsMatch for order-independence)
	assert.ElementsMatch(t, before.Packages.Formulae, after.Packages.Formulae,
		"dry-run should not change installed formulae")
	assert.ElementsMatch(t, before.Packages.Casks, after.Packages.Casks,
		"dry-run should not change installed casks")
	assert.ElementsMatch(t, before.Packages.Npm, after.Packages.Npm,
		"dry-run should not change installed npm packages")
}

func TestSmoke_VersionMatchesBuild(t *testing.T) {
	binary := testutil.BuildTestBinary(t)

	cmd := exec.Command(binary, "version")
	output, err := cmd.CombinedOutput()
	outStr := string(output)

	require.NoError(t, err, "version command should succeed")
	assert.Contains(t, outStr, "OpenBoot v", "version output should contain version prefix")
}
