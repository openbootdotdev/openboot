//go:build e2e

package e2e

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/openbootdotdev/openboot/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestE2E_InstallSinglePackage_JQ(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping real installation test in short mode")
	}

	// Given: system does not have jq installed
	testutil.EnsurePackageNotInstalled(t, "jq")
	binary := testutil.BuildTestBinary(t)

	// When: we install jq via openboot
	cmd := exec.Command(binary, "--packages-only", "--silent", "--preset", "minimal")
	cmd.Env = append(os.Environ(),
		"OPENBOOT_GIT_NAME=Test User",
		"OPENBOOT_GIT_EMAIL=test@example.com",
	)

	tmpConfig := createTempConfig(t, `{
		"packages": {
			"brew": ["jq"]
		}
	}`)
	defer os.Remove(tmpConfig)

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
	if testing.Short() {
		t.Skip("skipping real installation test in short mode")
	}

	packages := []string{"bat", "fd"}

	// Given: packages are not installed
	for _, pkg := range packages {
		testutil.EnsurePackageNotInstalled(t, pkg)
	}
	binary := testutil.BuildTestBinary(t)

	// When: we install multiple packages
	tmpConfig := createTempConfig(t, `{
		"packages": {
			"brew": ["bat", "fd"]
		}
	}`)
	defer os.Remove(tmpConfig)

	cmd := exec.Command(binary, "--packages-only", "--silent", "--preset", "minimal")
	cmd.Env = append(os.Environ(),
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

func TestE2E_SnapshotRestoreRealPackages(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping real installation test in short mode")
	}

	binary := testutil.BuildTestBinary(t)
	testPkg := "ripgrep"

	// Given: ripgrep is installed
	if !testutil.IsPackageInstalled(testPkg) {
		cmd := exec.Command("brew", "install", testPkg)
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

	// Then: snapshot should contain ripgrep
	packages, ok := snapshot["packages"].(map[string]interface{})
	require.True(t, ok, "snapshot should have packages field")

	brew, ok := packages["brew"].([]interface{})
	require.True(t, ok, "snapshot should have brew packages")

	foundRipgrep := false
	for _, pkg := range brew {
		if pkgStr, ok := pkg.(string); ok && strings.Contains(pkgStr, "ripgrep") {
			foundRipgrep = true
			break
		}
	}
	assert.True(t, foundRipgrep, "snapshot should contain ripgrep")

	// Save snapshot to file
	tmpDir := t.TempDir()
	snapshotPath := filepath.Join(tmpDir, "test-snapshot.json")
	err = os.WriteFile(snapshotPath, []byte(snapshotJSON), 0644)
	require.NoError(t, err)

	// When: we uninstall ripgrep and restore from snapshot
	testutil.UninstallPackage(t, testPkg)
	assert.False(t, testutil.IsPackageInstalled(testPkg), "ripgrep should be uninstalled")

	// Note: Actual restore would require implementing --restore flag
	// For now, we verify the snapshot format is correct
	content, err := os.ReadFile(snapshotPath)
	require.NoError(t, err)
	assert.Greater(t, len(content), 100, "snapshot should have substantial content")
}

func TestE2E_InstallWithInvalidPackage(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping real installation test in short mode")
	}

	binary := testutil.BuildTestBinary(t)
	invalidPkg := "this-package-definitely-does-not-exist-12345"

	// Given: we have a config with an invalid package
	tmpConfig := createTempConfig(t, `{
		"packages": {
			"brew": ["`+invalidPkg+`"]
		}
	}`)
	defer os.Remove(tmpConfig)

	// When: we try to install it
	cmd := exec.Command(binary, "--packages-only", "--silent", "--preset", "minimal")
	cmd.Env = append(os.Environ(),
		"OPENBOOT_GIT_NAME=Test User",
		"OPENBOOT_GIT_EMAIL=test@example.com",
	)

	output, err := cmd.CombinedOutput()
	outStr := string(output)
	t.Logf("Error output: %s", outStr)

	// Then: command should fail gracefully
	// Note: OpenBoot may continue with valid packages, so we just check for error indication
	assert.True(t, err != nil || strings.Contains(outStr, "error") || strings.Contains(outStr, "failed"),
		"should indicate error for invalid package")
}

func TestE2E_DryRunDoesNotInstall(t *testing.T) {
	binary := testutil.BuildTestBinary(t)
	testPkg := "cowsay"

	// Given: cowsay is not installed
	testutil.EnsurePackageNotInstalled(t, testPkg)

	// When: we run with --dry-run
	tmpConfig := createTempConfig(t, `{
		"packages": {
			"brew": ["`+testPkg+`"]
		}
	}`)
	defer os.Remove(tmpConfig)

	cmd := exec.Command(binary, "--dry-run", "--packages-only", "--silent", "--preset", "minimal")
	cmd.Env = append(os.Environ(),
		"OPENBOOT_GIT_NAME=Test User",
		"OPENBOOT_GIT_EMAIL=test@example.com",
	)

	output, err := cmd.CombinedOutput()
	t.Logf("Dry-run output: %s", string(output))

	// Then: cowsay should still not be installed
	assert.NoError(t, err, "dry-run should succeed")
	assert.False(t, testutil.IsPackageInstalled(testPkg), "dry-run should not actually install packages")
}

func TestE2E_BrewUpdateBeforeInstall(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping brew update test in short mode")
	}

	binary := testutil.BuildTestBinary(t)

	// Given: we request brew update
	cmd := exec.Command(binary, "--update", "--dry-run", "--packages-only", "--silent", "--preset", "minimal")
	cmd.Env = append(os.Environ(),
		"OPENBOOT_GIT_NAME=Test User",
		"OPENBOOT_GIT_EMAIL=test@example.com",
	)

	start := time.Now()
	output, err := cmd.CombinedOutput()
	duration := time.Since(start)

	t.Logf("Update output: %s", string(output))
	t.Logf("Duration: %v", duration)

	// Then: command should succeed (update may happen or skip if recent)
	assert.NoError(t, err, "update command should succeed")
}

func TestE2E_GitConfigSetup(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping git config test in short mode")
	}

	binary := testutil.BuildTestBinary(t)
	testName := "Test E2E User"
	testEmail := "e2e-test@example.com"

	// Given: we have test git credentials
	cmd := exec.Command(binary, "--packages-only", "--silent", "--preset", "minimal")
	cmd.Env = append(os.Environ(),
		"OPENBOOT_GIT_NAME="+testName,
		"OPENBOOT_GIT_EMAIL="+testEmail,
	)

	// When: we run openboot
	output, err := cmd.CombinedOutput()
	t.Logf("Output: %s", string(output))

	// Then: command should handle git config
	assert.NoError(t, err, "should succeed with git config")

	// Verify git config is accessible (system-level test)
	gitNameCheck := exec.Command("git", "config", "--global", "user.name")
	nameOutput, _ := gitNameCheck.Output()
	t.Logf("Git user.name: %s", string(nameOutput))

	gitEmailCheck := exec.Command("git", "config", "--global", "user.email")
	emailOutput, _ := gitEmailCheck.Output()
	t.Logf("Git user.email: %s", string(emailOutput))
}

func createTempConfig(t *testing.T, jsonContent string) string {
	tmpFile, err := os.CreateTemp("", "openboot-config-*.json")
	require.NoError(t, err)

	_, err = tmpFile.WriteString(jsonContent)
	require.NoError(t, err)

	err = tmpFile.Close()
	require.NoError(t, err)

	return tmpFile.Name()
}
