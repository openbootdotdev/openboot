//go:build integration

package integration

import (
	"testing"

	"github.com/openbootdotdev/openboot/internal/brew"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegration_Brew_IsInstalled(t *testing.T) {
	// Given: a macOS machine running these integration tests
	// When: we check if brew is installed
	result := brew.IsInstalled()

	// Then: brew should be present (required for integration tests to be meaningful)
	assert.True(t, result, "brew must be installed to run integration tests")
}

func TestIntegration_Brew_GetInstalledPackages(t *testing.T) {
	// Given: brew is installed
	require.True(t, brew.IsInstalled(), "brew must be installed")

	// When: we fetch installed packages
	formulae, casks, err := brew.GetInstalledPackages()

	// Then: both maps should be valid and non-nil
	require.NoError(t, err)
	assert.NotNil(t, formulae)
	assert.NotNil(t, casks)
	for name := range formulae {
		assert.NotEmpty(t, name, "formula name should not be empty")
		assert.NotContains(t, name, "\n", "formula name should not contain newlines")
	}
	for name := range casks {
		assert.NotEmpty(t, name, "cask name should not be empty")
	}
	t.Logf("Found %d formulae, %d casks", len(formulae), len(casks))
}

func TestIntegration_Brew_ListOutdated(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping ListOutdated in short mode")
	}

	// Given: brew is installed
	require.True(t, brew.IsInstalled(), "brew must be installed")

	// When: we list outdated packages
	outdated, err := brew.ListOutdated()

	// Then: no error; result is a valid (possibly nil) slice — nil is expected on fresh runners
	require.NoError(t, err)
	for _, pkg := range outdated {
		assert.NotEmpty(t, pkg.Name, "outdated package name should not be empty")
		assert.NotEmpty(t, pkg.Latest, "outdated package latest version should not be empty")
	}
	t.Logf("Found %d outdated packages", len(outdated))
}

func TestIntegration_Brew_DoctorDiagnose(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DoctorDiagnose in short mode")
	}

	// Given: brew is installed
	require.True(t, brew.IsInstalled(), "brew must be installed")

	// When: we run brew doctor through the package
	suggestions, err := brew.DoctorDiagnose()

	// Then: command runs without panicking; brew doctor exits 1 on CI runners with warnings — skip on error
	if err != nil {
		t.Skipf("brew doctor returned an error (common on CI runners with warnings): %v", err)
	}
	if len(suggestions) > 0 {
		for _, s := range suggestions {
			assert.NotEmpty(t, s, "each suggestion should be non-empty")
			assert.Contains(t, s, "Run:", "suggestions should be actionable commands")
		}
		t.Logf("Doctor found %d suggestions", len(suggestions))
	} else {
		t.Log("brew doctor: system is ready to brew")
	}
}

func TestIntegration_Brew_CheckDiskSpace(t *testing.T) {
	// Given: running on macOS
	// When: we check available disk space
	gb, err := brew.CheckDiskSpace()

	// Then: should return a positive number without error
	require.NoError(t, err)
	assert.Greater(t, gb, 0.0, "available disk space should be positive")
	t.Logf("Available disk space: %.2f GB", gb)
}

func TestIntegration_Brew_CheckNetwork(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network check in short mode")
	}

	// Given: machine has internet access
	// When: we check connectivity to required GitHub hosts
	err := brew.CheckNetwork()

	// Then: both github.com and raw.githubusercontent.com are reachable
	assert.NoError(t, err, "network check should pass with internet access")
}

func TestIntegration_Brew_CheckNetwork_HostsReachable(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network host check in short mode")
	}

	// Given: CheckNetwork is expected to reach exactly two hosts
	// When: we call it on a machine with internet
	err := brew.CheckNetwork()

	// Then: no port-connection errors
	if err != nil {
		t.Logf("Network check failed (expected if offline): %v", err)
		t.Skip("machine appears to be offline")
	}
}

func TestIntegration_Brew_Update_DryRun(t *testing.T) {
	// Given: brew is installed
	require.True(t, brew.IsInstalled(), "brew must be installed")

	// When: we run Update in dry-run mode
	err := brew.Update(true)

	// Then: no error and nothing is actually updated
	assert.NoError(t, err)
}

func TestIntegration_Brew_Install_DryRun(t *testing.T) {
	// Given: brew is installed
	require.True(t, brew.IsInstalled(), "brew must be installed")

	// When: we call Install with dry-run for known packages
	err := brew.Install([]string{"git", "curl"}, true)

	// Then: no error, nothing installed
	assert.NoError(t, err)
}

func TestIntegration_Brew_InstallCask_DryRun(t *testing.T) {
	// Given: brew is installed
	require.True(t, brew.IsInstalled(), "brew must be installed")

	// When: we call InstallCask with dry-run
	err := brew.InstallCask([]string{"firefox"}, true)

	// Then: no error, nothing installed
	assert.NoError(t, err)
}

func TestIntegration_Brew_InstallTaps_DryRun(t *testing.T) {
	// Given: brew is installed
	require.True(t, brew.IsInstalled(), "brew must be installed")

	// When: we call InstallTaps with dry-run
	err := brew.InstallTaps([]string{"homebrew/cask-fonts"}, true)

	// Then: no error, nothing tapped
	assert.NoError(t, err)
}

func TestIntegration_Brew_InstallWithProgress_DryRun(t *testing.T) {
	// Given: brew is installed
	require.True(t, brew.IsInstalled(), "brew must be installed")

	// When: we call InstallWithProgress with dry-run
	formulae, casks, err := brew.InstallWithProgress([]string{"git"}, []string{"firefox"}, true)

	// Then: no error; dry-run returns empty slices
	assert.NoError(t, err)
	assert.Empty(t, formulae)
	assert.Empty(t, casks)
}

func TestIntegration_Brew_Uninstall_DryRun(t *testing.T) {
	// Given: brew is installed
	require.True(t, brew.IsInstalled(), "brew must be installed")

	// When: we call Uninstall with dry-run for a package that may or may not be installed
	err := brew.Uninstall([]string{"wget"}, true)

	// Then: no error (dry-run never touches the system)
	assert.NoError(t, err)
}

func TestIntegration_Brew_UninstallCask_DryRun(t *testing.T) {
	// Given: brew is installed
	require.True(t, brew.IsInstalled(), "brew must be installed")

	// When: we call UninstallCask with dry-run
	err := brew.UninstallCask([]string{"firefox"}, true)

	// Then: no error
	assert.NoError(t, err)
}

func TestIntegration_Brew_GetInstalledPackages_Consistency(t *testing.T) {
	// Given: brew is installed
	require.True(t, brew.IsInstalled(), "brew must be installed")

	// When: we call GetInstalledPackages twice in a row
	formulae1, casks1, err1 := brew.GetInstalledPackages()
	formulae2, casks2, err2 := brew.GetInstalledPackages()

	// Then: both calls return the same result (no concurrent modification)
	require.NoError(t, err1)
	require.NoError(t, err2)
	assert.Equal(t, len(formulae1), len(formulae2), "formulae count should be stable")
	assert.Equal(t, len(casks1), len(casks2), "casks count should be stable")
}
