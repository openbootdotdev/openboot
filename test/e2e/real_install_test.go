//go:build e2e && vm

package e2e

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/openbootdotdev/openboot/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestE2E_InstallSinglePackage_JQ(t *testing.T) {
	// Given: system does not have jq installed
	testutil.UninstallPackage(t, "jq")
	if testutil.IsPackageInstalled("jq") {
		t.Skip("jq cannot be uninstalled (likely a dependency); skipping")
	}
	binary := testutil.BuildTestBinary(t)

	// When: we install jq via openboot (minimal preset includes jq)
	cmd := exec.Command(binary, "install", "--packages-only", "--silent", "--preset", "minimal")
	cmd.Env = append(os.Environ(),
		"OPENBOOT_GIT_NAME=Test User",
		"OPENBOOT_GIT_EMAIL=test@example.com",
	)

	output, err := cmd.CombinedOutput()
	t.Logf("Install output: %s", string(output))

	// Then: jq should be installed and executable
	assert.NoError(t, err, "installation should succeed")
	assert.True(t, testutil.IsPackageInstalled("jq"), "jq should be installed")

	jqPath, err := exec.Command("which", "jq").Output()
	require.NoError(t, err, "jq should be in PATH")
	assert.Contains(t, string(jqPath), "jq")

	jqVersion, err := exec.Command("jq", "--version").Output()
	require.NoError(t, err, "jq should be executable")
	assert.Contains(t, string(jqVersion), "jq-")

	// Cleanup
	testutil.UninstallPackage(t, "jq")
}

func TestE2E_InstallMultiplePackages(t *testing.T) {
	packages := []string{"bat", "fd"}

	// Given: packages are not installed
	for _, pkg := range packages {
		testutil.EnsurePackageNotInstalled(t, pkg)
	}
	binary := testutil.BuildTestBinary(t)

	// When: we install multiple packages via the minimal preset (includes bat + fd)
	cmd := exec.Command(binary, "install", "--packages-only", "--silent", "--preset", "minimal")
	cmd.Env = append(os.Environ(),
		"PATH=/opt/homebrew/bin:/opt/homebrew/sbin:"+os.Getenv("PATH"),
		"OPENBOOT_GIT_NAME=Test User",
		"OPENBOOT_GIT_EMAIL=test@example.com",
	)

	output, err := cmd.CombinedOutput()
	t.Logf("Install output: %s", string(output))

	// Then: all packages should be installed
	assert.NoError(t, err, "installation should succeed")

	for _, pkg := range packages {
		assert.True(t, testutil.IsPackageInstalled(pkg), "%s should be installed", pkg)

		pkgPath, err := exec.Command("which", pkg).Output()
		require.NoError(t, err, "%s should be in PATH", pkg)
		assert.Contains(t, string(pkgPath), pkg)
	}

	// Cleanup
	for _, pkg := range packages {
		testutil.UninstallPackage(t, pkg)
	}
}

func TestE2E_SnapshotCapture_RecordsInstalledPackage(t *testing.T) {
	binary := testutil.BuildTestBinary(t)
	testPkg := "ripgrep"

	// Given: ripgrep is installed
	if !testutil.IsPackageInstalled(testPkg) {
		cmd := exec.Command("/opt/homebrew/bin/brew", "install", testPkg)
		require.NoError(t, cmd.Run(), "test setup: should install ripgrep")
	}

	// When: we capture a snapshot
	cmd := exec.Command(binary, "snapshot", "--json")
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	require.NoError(t, err, "snapshot capture should succeed, stderr: %s", stderr.String())

	snapshotJSON := stdout.String()
	var snapshot map[string]interface{}
	err = json.Unmarshal([]byte(snapshotJSON), &snapshot)
	require.NoError(t, err, "snapshot should be valid JSON")

	// Then: snapshot should contain ripgrep in formulae
	packages, ok := snapshot["packages"].(map[string]interface{})
	require.True(t, ok, "snapshot should have packages field")

	brew, ok := packages["formulae"].([]interface{})
	require.True(t, ok, "snapshot should have formulae packages")

	foundRipgrep := false
	for _, pkg := range brew {
		if pkgStr, ok := pkg.(string); ok && strings.Contains(pkgStr, "ripgrep") {
			foundRipgrep = true
			break
		}
	}
	assert.True(t, foundRipgrep, "snapshot should contain ripgrep")
}

