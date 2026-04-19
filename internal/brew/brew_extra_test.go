package brew

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Untap
// ---------------------------------------------------------------------------

func TestUntap_Empty(t *testing.T) {
	err := Untap([]string{}, false)
	assert.NoError(t, err)
}

func TestUntap_DryRun(t *testing.T) {
	err := Untap([]string{"homebrew/cask", "hashicorp/tap"}, true)
	assert.NoError(t, err)
}

func TestUntap_Success(t *testing.T) {
	var called []string
	withFakeBrew(t, func(args []string) ([]byte, error) {
		if len(args) >= 2 && args[0] == "untap" {
			called = append(called, args[1])
		}
		return nil, nil
	})

	err := Untap([]string{"homebrew/cask", "hashicorp/tap"}, false)
	require.NoError(t, err)
	assert.Equal(t, []string{"homebrew/cask", "hashicorp/tap"}, called)
}

func TestUntap_Failure(t *testing.T) {
	withFakeBrew(t, func(args []string) ([]byte, error) {
		return []byte("Error: No such tap"), errors.New("exit 1")
	})

	err := Untap([]string{"bad/tap"}, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tap(s) failed to remove")
}

func TestUntap_PartialFailure(t *testing.T) {
	withFakeBrew(t, func(args []string) ([]byte, error) {
		if len(args) >= 2 && args[1] == "bad/tap" {
			return []byte("Error: No such tap"), errors.New("exit 1")
		}
		return nil, nil
	})

	err := Untap([]string{"good/tap", "bad/tap"}, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "1 tap(s) failed")
}

// ---------------------------------------------------------------------------
// GetInstalledPackages – error paths
// ---------------------------------------------------------------------------

func TestGetInstalledPackages_FormulaError(t *testing.T) {
	withFakeBrew(t, func(args []string) ([]byte, error) {
		if len(args) >= 2 && args[0] == "list" && args[1] == "--formula" {
			return nil, errors.New("brew list --formula failed")
		}
		// cask succeeds
		return []byte("firefox\n"), nil
	})

	_, _, err := GetInstalledPackages()
	require.Error(t, err)
}

func TestGetInstalledPackages_CaskError(t *testing.T) {
	withFakeBrew(t, func(args []string) ([]byte, error) {
		if len(args) >= 2 && args[0] == "list" && args[1] == "--cask" {
			return nil, errors.New("brew list --cask failed")
		}
		// formula succeeds
		return []byte("git\n"), nil
	})

	_, _, err := GetInstalledPackages()
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// ListOutdated – error paths
// ---------------------------------------------------------------------------

func TestListOutdated_RunnerError(t *testing.T) {
	withFakeBrew(t, func(args []string) ([]byte, error) {
		return nil, errors.New("network error")
	})

	_, err := ListOutdated()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "brew outdated")
}

func TestListOutdated_InvalidJSON(t *testing.T) {
	withFakeBrew(t, func(args []string) ([]byte, error) {
		if len(args) >= 2 && args[0] == "outdated" {
			return []byte("not-json"), nil
		}
		return nil, nil
	})

	_, err := ListOutdated()
	require.Error(t, err)
}

func TestListOutdated_EmptyResult(t *testing.T) {
	withFakeBrew(t, func(args []string) ([]byte, error) {
		if len(args) >= 2 && args[0] == "outdated" {
			return []byte(`{"formulae":[],"casks":[]}`), nil
		}
		return nil, nil
	})

	pkgs, err := ListOutdated()
	require.NoError(t, err)
	assert.Empty(t, pkgs)
}

func TestListOutdated_NoInstalledVersions(t *testing.T) {
	// Ensure an entry with empty installed_versions still maps correctly.
	withFakeBrew(t, func(args []string) ([]byte, error) {
		if len(args) >= 1 && args[0] == "outdated" {
			return []byte(`{"formulae":[{"name":"git","installed_versions":[],"current_version":"2.40.0"}],"casks":[]}`), nil
		}
		return nil, nil
	})

	pkgs, err := ListOutdated()
	require.NoError(t, err)
	require.Len(t, pkgs, 1)
	assert.Equal(t, "git", pkgs[0].Name)
	assert.Equal(t, "", pkgs[0].Current)
	assert.Equal(t, "2.40.0", pkgs[0].Latest)
}

// ---------------------------------------------------------------------------
// identityMap (internal helper)
// ---------------------------------------------------------------------------

func TestIdentityMap(t *testing.T) {
	names := []string{"a", "b", "c"}
	m := identityMap(names)
	assert.Equal(t, map[string]string{"a": "a", "b": "b", "c": "c"}, m)
}

func TestIdentityMap_Empty(t *testing.T) {
	m := identityMap([]string{})
	assert.Empty(t, m)
}

// ---------------------------------------------------------------------------
// PreInstallChecks – success path with stubbed network and disk
// ---------------------------------------------------------------------------

func TestPreInstallChecks_NetworkFails(t *testing.T) {
	orig := checkNetworkFunc
	checkNetworkFunc = func() error { return errors.New("no route to host") }
	t.Cleanup(func() { checkNetworkFunc = orig })

	err := PreInstallChecks(5)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "network check failed")
}

func TestPreInstallChecks_NetworkOK(t *testing.T) {
	orig := checkNetworkFunc
	checkNetworkFunc = func() error { return nil }
	t.Cleanup(func() { checkNetworkFunc = orig })

	// For the brew update sub-call we need a fake brew on PATH.
	setupFakeBrew(t, "#!/bin/sh\nexit 0\n")

	// Should not error (disk space should be available on a dev machine).
	err := PreInstallChecks(1)
	assert.NoError(t, err)
}

// ---------------------------------------------------------------------------
// IsInstalled — brew binary presence check
// ---------------------------------------------------------------------------

func TestIsInstalled_BinaryOnPath(t *testing.T) {
	// Install a fake brew on PATH so LookPath finds it.
	setupFakeBrew(t, "#!/bin/sh\nexit 0\n")
	assert.True(t, IsInstalled())
}

func TestIsInstalled_BinaryNotOnPath(t *testing.T) {
	// Override PATH to an empty temp dir so brew is not found.
	t.Setenv("PATH", t.TempDir())
	assert.False(t, IsInstalled())
}

// ---------------------------------------------------------------------------
// IsInstalled / IsCaskInstalled — derived from GetInstalledPackages mock
// ---------------------------------------------------------------------------

func TestGetInstalledPackages_IsInstalledLookup(t *testing.T) {
	withFakeBrew(t, func(args []string) ([]byte, error) {
		if len(args) >= 2 && args[0] == "list" && args[1] == "--formula" {
			return []byte("git\ncurl\n"), nil
		}
		if len(args) >= 2 && args[0] == "list" && args[1] == "--cask" {
			return []byte("firefox\nslack\n"), nil
		}
		return nil, nil
	})

	formulae, casks, err := GetInstalledPackages()
	require.NoError(t, err)

	// Installed formulae
	assert.True(t, formulae["git"], "git should be installed")
	assert.True(t, formulae["curl"], "curl should be installed")
	assert.False(t, formulae["wget"], "wget should not be installed")

	// Installed casks
	assert.True(t, casks["firefox"], "firefox should be installed")
	assert.False(t, casks["chrome"], "chrome should not be installed")
}
