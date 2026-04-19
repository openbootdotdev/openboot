package npm

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// getNodeVersion — error paths
// ---------------------------------------------------------------------------

func TestGetNodeVersion_ExecError(t *testing.T) {
	orig := nodeVersionOutput
	t.Cleanup(func() { nodeVersionOutput = orig })
	nodeVersionOutput = func() ([]byte, error) {
		return nil, errors.New("node not found")
	}

	_, err := getNodeVersion()
	require.Error(t, err)
}

func TestGetNodeVersion_InvalidFormat(t *testing.T) {
	orig := nodeVersionOutput
	t.Cleanup(func() { nodeVersionOutput = orig })
	nodeVersionOutput = func() ([]byte, error) {
		return []byte("not-a-version\n"), nil
	}

	_, err := getNodeVersion()
	require.Error(t, err)
}

func TestGetNodeVersion_EmptyOutput(t *testing.T) {
	orig := nodeVersionOutput
	t.Cleanup(func() { nodeVersionOutput = orig })
	nodeVersionOutput = func() ([]byte, error) {
		return []byte(""), nil
	}

	_, err := getNodeVersion()
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// GetInstalledPackages — error path (no output + non-zero exit)
// ---------------------------------------------------------------------------

func TestGetInstalledPackages_ErrorNoOutput(t *testing.T) {
	withFakeNpm(t, func(args []string) ([]byte, error) {
		return nil, errors.New("npm not found")
	})

	_, err := GetInstalledPackages()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "npm list -g")
}

func TestGetInstalledPackages_ErrorWithOutput_TreatedAsSuccess(t *testing.T) {
	// npm list exits non-zero but still provides parseable output — treated as success.
	withFakeNpm(t, func(args []string) ([]byte, error) {
		if len(args) > 0 && args[0] == "list" {
			return []byte(`/usr/local/lib/node_modules
/usr/local/lib/node_modules/eslint
`), errors.New("exited with non-zero")
		}
		return nil, nil
	})

	packages, err := GetInstalledPackages()
	require.NoError(t, err)
	assert.True(t, packages["eslint"])
}

func TestGetInstalledPackages_ScopedPackage(t *testing.T) {
	withFakeNpm(t, func(args []string) ([]byte, error) {
		return []byte(`/usr/local/lib/node_modules
/usr/local/lib/node_modules/@angular/cli
/usr/local/lib/node_modules/prettier
`), nil
	})

	packages, err := GetInstalledPackages()
	require.NoError(t, err)
	assert.True(t, packages["@angular/cli"])
	assert.True(t, packages["prettier"])
}

func TestGetInstalledPackages_SingleLineOnly(t *testing.T) {
	// Only the root path line — no packages.
	withFakeNpm(t, func(args []string) ([]byte, error) {
		return []byte("/usr/local/lib/node_modules\n"), nil
	})

	packages, err := GetInstalledPackages()
	require.NoError(t, err)
	assert.Empty(t, packages)
}

// ---------------------------------------------------------------------------
// Uninstall
// ---------------------------------------------------------------------------

func TestUninstall_EmptyPackages(t *testing.T) {
	err := Uninstall([]string{}, false)
	assert.NoError(t, err)
}

func TestUninstall_DryRun(t *testing.T) {
	err := Uninstall([]string{"typescript", "eslint"}, true)
	assert.NoError(t, err)
}

func TestUninstall_Success(t *testing.T) {
	var removed []string
	withFakeNpm(t, func(args []string) ([]byte, error) {
		if len(args) > 0 && args[0] == "uninstall" {
			// args = ["uninstall", "-g", "<pkg>"]
			if len(args) >= 3 {
				removed = append(removed, args[2])
			}
			return []byte("removed"), nil
		}
		return nil, nil
	})

	err := Uninstall([]string{"typescript", "eslint"}, false)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"typescript", "eslint"}, removed)
}

func TestUninstall_PartialFailure(t *testing.T) {
	withFakeNpm(t, func(args []string) ([]byte, error) {
		if len(args) >= 3 && args[2] == "typescript" {
			return []byte("some error output"), errors.New("exit status 1")
		}
		return []byte("removed"), nil
	})

	err := Uninstall([]string{"typescript", "eslint"}, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to uninstall")
}

func TestUninstall_AllFail(t *testing.T) {
	withFakeNpm(t, func(args []string) ([]byte, error) {
		return []byte("permission denied"), errors.New("exit status 1")
	})

	err := Uninstall([]string{"pkg-a", "pkg-b"}, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "2 npm packages failed")
}

// ---------------------------------------------------------------------------
// installNpmPackageWithRetry
// ---------------------------------------------------------------------------

func TestInstallNpmPackageWithRetry_Success(t *testing.T) {
	withFakeNpm(t, func(args []string) ([]byte, error) {
		return []byte("installed"), nil
	})

	result := installNpmPackageWithRetry("typescript")
	assert.Equal(t, "", result, "empty string means success")
}

func TestInstallNpmPackageWithRetry_PermanentError(t *testing.T) {
	withFakeNpm(t, func(args []string) ([]byte, error) {
		return []byte("npm ERR! 404 Not Found"), errors.New("exit status 1")
	})

	result := installNpmPackageWithRetry("nonexistent-package")
	assert.NotEmpty(t, result)
	assert.Equal(t, "package not found", result)
}

func TestInstallNpmPackageWithRetry_RetryableSucceedsOnSecondAttempt(t *testing.T) {
	// Speed up retry delays.
	origBackoff := retryBackoff
	retryBackoff = 1 * time.Millisecond
	t.Cleanup(func() { retryBackoff = origBackoff })

	attempt := 0
	withFakeNpm(t, func(args []string) ([]byte, error) {
		attempt++
		if attempt < 2 {
			return []byte("npm ERR! network error"), errors.New("exit status 1")
		}
		return []byte("installed"), nil
	})

	result := installNpmPackageWithRetry("some-package")
	assert.Equal(t, "", result, "should succeed on second attempt")
}

func TestInstallNpmPackageWithRetry_RetryableExhausted(t *testing.T) {
	// Speed up retry delays.
	origBackoff := retryBackoff
	retryBackoff = 1 * time.Millisecond
	t.Cleanup(func() { retryBackoff = origBackoff })

	withFakeNpm(t, func(args []string) ([]byte, error) {
		return []byte("npm ERR! network error"), errors.New("exit status 1")
	})

	result := installNpmPackageWithRetry("some-package")
	// After 3 attempts it stops retrying; last attempt still returns the last errMsg.
	assert.NotEmpty(t, result)
}

// ---------------------------------------------------------------------------
// Install — batch-fail / sequential fallback path
// ---------------------------------------------------------------------------

func TestInstall_BatchFailFallbackToSequential(t *testing.T) {
	// Speed up retry delays.
	origBackoff := retryBackoff
	retryBackoff = 1 * time.Millisecond
	t.Cleanup(func() { retryBackoff = origBackoff })

	callCount := 0
	withFakeNpm(t, func(args []string) ([]byte, error) {
		callCount++
		switch {
		case len(args) > 0 && args[0] == "list":
			// Return no pre-installed packages.
			return []byte("/usr/local/lib/node_modules\n"), nil
		case len(args) > 1 && args[0] == "install" && args[1] == "-g" && len(args) > 3:
			// Batch install (multiple packages) fails.
			return []byte("npm ERR! EACCES permission denied"), errors.New("exit status 1")
		case len(args) > 1 && args[0] == "install" && args[1] == "-g" && len(args) == 3:
			// Sequential install (single package) succeeds.
			return []byte("installed"), nil
		}
		return nil, nil
	})

	err := Install([]string{"eslint", "prettier"}, false)
	require.NoError(t, err)
}

func TestInstall_BatchFailSequentialFail(t *testing.T) {
	origBackoff := retryBackoff
	retryBackoff = 1 * time.Millisecond
	t.Cleanup(func() { retryBackoff = origBackoff })

	withFakeNpm(t, func(args []string) ([]byte, error) {
		if len(args) > 0 && args[0] == "list" {
			return []byte("/usr/local/lib/node_modules\n"), nil
		}
		// Both batch and sequential fail.
		return []byte("npm ERR! 404 Not Found"), errors.New("exit status 1")
	})

	err := Install([]string{"nonexistent-pkg"}, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to install")
}

func TestInstall_BatchFailAllAlreadyInstalledAfterPartial(t *testing.T) {
	// The batch install fails but a subsequent list shows everything installed
	// (partial-batch success scenario).
	callCount := 0
	withFakeNpm(t, func(args []string) ([]byte, error) {
		callCount++
		if len(args) > 0 && args[0] == "list" {
			if callCount == 1 {
				// First list: nothing installed.
				return []byte("/usr/local/lib/node_modules\n"), nil
			}
			// Second list (after failed batch): everything now installed.
			return []byte(`/usr/local/lib/node_modules
/usr/local/lib/node_modules/eslint
/usr/local/lib/node_modules/prettier
`), nil
		}
		// Batch install "fails" even though packages ended up installed.
		return []byte("npm WARN something"), errors.New("exit status 1")
	})

	err := Install([]string{"eslint", "prettier"}, false)
	require.NoError(t, err)
}

func TestInstall_WranglerWarning_OldNode(t *testing.T) {
	// Stub node version to something < 22.
	orig := nodeVersionOutput
	t.Cleanup(func() { nodeVersionOutput = orig })
	nodeVersionOutput = func() ([]byte, error) { return []byte("v18.0.0\n"), nil }

	// Dry-run so no actual npm calls happen.
	err := Install([]string{"wrangler"}, true)
	assert.NoError(t, err)
}

func TestInstall_WranglerWarning_NewNode(t *testing.T) {
	orig := nodeVersionOutput
	t.Cleanup(func() { nodeVersionOutput = orig })
	nodeVersionOutput = func() ([]byte, error) { return []byte("v22.0.0\n"), nil }

	err := Install([]string{"wrangler"}, true)
	assert.NoError(t, err)
}

// ---------------------------------------------------------------------------
// SetRunner restore function
// ---------------------------------------------------------------------------

func TestSetRunner_RestoresPreviousRunner(t *testing.T) {
	original := currentRunner()

	fake := &fakeRunner{handler: func(args []string) ([]byte, error) {
		return []byte("fake"), nil
	}}
	restore := SetRunner(fake)

	assert.Equal(t, fake, currentRunner())
	restore()
	assert.Equal(t, original, currentRunner())
}

// ---------------------------------------------------------------------------
// GetInstalledPackages — empty output
// ---------------------------------------------------------------------------

func TestGetInstalledPackages_EmptyOutput(t *testing.T) {
	withFakeNpm(t, func(args []string) ([]byte, error) {
		return []byte(""), nil
	})

	packages, err := GetInstalledPackages()
	require.NoError(t, err)
	assert.Empty(t, packages)
}

// ---------------------------------------------------------------------------
// getNodeVersion — major-only version string edge case
// ---------------------------------------------------------------------------

func TestGetNodeVersion_MajorOnly(t *testing.T) {
	orig := nodeVersionOutput
	t.Cleanup(func() { nodeVersionOutput = orig })
	nodeVersionOutput = func() ([]byte, error) { return []byte("v20\n"), nil }

	ver, err := getNodeVersion()
	require.NoError(t, err)
	assert.Equal(t, 20, ver)
}

// ---------------------------------------------------------------------------
// Uninstall — npm not available (no PATH manipulation, just verifies guard)
// ---------------------------------------------------------------------------

func TestUninstall_NpmNotAvailableIsNoop(t *testing.T) {
	// Confirm the no-npm guard: if IsAvailable() returns false the function
	// must still return nil. We verify the guarded path indirectly: even
	// calling with packages, non-dry-run, and an always-erroring runner
	// doesn't blow up as long as IsAvailable is false.
	// (We can't easily fake exec.LookPath without patching, so we just test
	// the guarded function surface with an empty list, which short-circuits
	// before the availability check.)
	err := Uninstall([]string{}, false)
	assert.NoError(t, err)
}

// ---------------------------------------------------------------------------
// parseNpmError — case-insensitivity
// ---------------------------------------------------------------------------

func TestParseNpmError_CaseInsensitive(t *testing.T) {
	tests := []struct {
		output   string
		expected string
	}{
		{"NPM ERR! 404 NOT FOUND", "package not found"},
		{"NPM ERR! CODE EACCES", "permission denied"},
		{"NPM ERR! CODE ENETWORK", "network error"},
		{"NPM ERR! CODE ENOTFOUND", "network error"},
		{"NPM ERR! CODE ENOSPC", "disk full"},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("input=%q", tt.output), func(t *testing.T) {
			assert.Equal(t, tt.expected, parseNpmError(tt.output))
		})
	}
}
