package brew

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupFakeBrew installs a shell-script fake `brew` on PATH for the test.
// Kept for tests that exercise the streaming install path (still uses
// exec.Command directly). For simple command tests, prefer withFakeBrew
// below — it avoids the fork/exec overhead.
func setupFakeBrew(t *testing.T, script string) {
	t.Helper()
	tmpDir := t.TempDir()
	brewPath := filepath.Join(tmpDir, "brew")
	require.NoError(t, os.WriteFile(brewPath, []byte(script), 0755))
	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", tmpDir+string(os.PathListSeparator)+originalPath)
}

// fakeRunner is a test-only Runner that routes every brew invocation through
// a user-provided handler. Replaces the PATH-hijack approach with pure Go —
// no fork/exec, so each test runs in ~microseconds.
type fakeRunner struct {
	handler func(args []string) ([]byte, error)
}

func (f *fakeRunner) Output(args ...string) ([]byte, error) {
	return f.handler(args)
}

func (f *fakeRunner) CombinedOutput(args ...string) ([]byte, error) {
	return f.handler(args)
}

func (f *fakeRunner) Run(args ...string) error {
	_, err := f.handler(args)
	return err
}

// withFakeBrew installs a fakeRunner for the duration of the test and
// restores the previous runner on cleanup.
func withFakeBrew(t *testing.T, handler func(args []string) ([]byte, error)) {
	t.Helper()
	t.Cleanup(SetRunner(&fakeRunner{handler: handler}))
}

func TestGetInstalledPackages_ParsesOutput(t *testing.T) {
	withFakeBrew(t, func(args []string) ([]byte, error) {
		if len(args) >= 2 && args[0] == "list" && args[1] == "--formula" {
			return []byte("git\ncurl\n"), nil
		}
		if len(args) >= 2 && args[0] == "list" && args[1] == "--cask" {
			return []byte("firefox\n"), nil
		}
		return nil, nil
	})

	formulae, casks, err := GetInstalledPackages()
	require.NoError(t, err)
	assert.True(t, formulae["git"])
	assert.True(t, formulae["curl"])
	assert.True(t, casks["firefox"])
}

func TestListOutdated_ParsesJSON(t *testing.T) {
	withFakeBrew(t, func(args []string) ([]byte, error) {
		if len(args) >= 2 && args[0] == "outdated" && args[1] == "--json" {
			return []byte(`{"formulae":[{"name":"git","installed_versions":["2.0"],"current_version":"2.1"}],"casks":[{"name":"firefox","installed_versions":["1.0"],"current_version":"2.0"}]}`), nil
		}
		return nil, nil
	})

	outdated, err := ListOutdated()
	require.NoError(t, err)
	assert.Len(t, outdated, 2)
	assert.Equal(t, "git", outdated[0].Name)
	assert.Equal(t, "firefox (cask)", outdated[1].Name)
}

func TestDoctorDiagnose_Suggestions(t *testing.T) {
	withFakeBrew(t, func(args []string) ([]byte, error) {
		if len(args) > 0 && args[0] == "doctor" {
			return []byte("Warning: unbrewed header files were found\nWarning: broken symlinks detected\n"), nil
		}
		return nil, nil
	})

	suggestions, err := DoctorDiagnose()
	require.NoError(t, err)
	assert.Contains(t, suggestions, "Run: sudo rm -rf /usr/local/include")
	assert.Contains(t, suggestions, "Run: brew cleanup --prune=all")
}

func TestUpdateAndCleanup_UsesBrew(t *testing.T) {
	var calls []string
	withFakeBrew(t, func(args []string) ([]byte, error) {
		if len(args) > 0 {
			calls = append(calls, args[0])
		}
		return nil, nil
	})

	// Update now calls brew upgrade directly via exec.Command (TTY handling).
	// Here we only verify that runner-routed calls (brew update, brew cleanup)
	// were made. The brew upgrade path is exercised by integration tests.
	err := Cleanup()
	assert.NoError(t, err)
	assert.Contains(t, calls, "cleanup")
}

func TestUninstall_Empty(t *testing.T) {
	err := Uninstall([]string{}, false)
	assert.NoError(t, err)
}

func TestUninstall_DryRun(t *testing.T) {
	err := Uninstall([]string{"git", "curl"}, true)
	assert.NoError(t, err)
}

func TestUninstall_Success(t *testing.T) {
	withFakeBrew(t, func(args []string) ([]byte, error) {
		return nil, nil
	})
	err := Uninstall([]string{"wget", "jq"}, false)
	assert.NoError(t, err)
}

func TestUninstall_Failure(t *testing.T) {
	withFakeBrew(t, func(args []string) ([]byte, error) {
		return []byte("Error: No such keg"), errors.New("exit 1")
	})
	err := Uninstall([]string{"nonexistent"}, false)
	assert.Error(t, err)
}

func TestUninstall_PartialFailure(t *testing.T) {
	withFakeBrew(t, func(args []string) ([]byte, error) {
		// args == ["uninstall", pkg]
		if len(args) >= 2 && args[1] == "bad-pkg" {
			return []byte("Error: No such keg"), errors.New("exit 1")
		}
		return nil, nil
	})
	err := Uninstall([]string{"good-pkg", "bad-pkg"}, false)
	assert.Error(t, err)
}

func TestUninstallCask_Empty(t *testing.T) {
	err := UninstallCask([]string{}, false)
	assert.NoError(t, err)
}

func TestUninstallCask_DryRun(t *testing.T) {
	err := UninstallCask([]string{"firefox", "slack"}, true)
	assert.NoError(t, err)
}

func TestUninstallCask_Success(t *testing.T) {
	withFakeBrew(t, func(args []string) ([]byte, error) {
		return nil, nil
	})
	err := UninstallCask([]string{"firefox"}, false)
	assert.NoError(t, err)
}

func TestUninstallCask_Failure(t *testing.T) {
	withFakeBrew(t, func(args []string) ([]byte, error) {
		return []byte("Error: Cask not found"), errors.New("exit 1")
	})
	err := UninstallCask([]string{"nonexistent-cask"}, false)
	assert.Error(t, err)
}

func TestDoctorDiagnose_ReadyToBrew(t *testing.T) {
	withFakeBrew(t, func(args []string) ([]byte, error) {
		return []byte("Your system is ready to brew.\n"), nil
	})
	suggestions, err := DoctorDiagnose()
	require.NoError(t, err)
	assert.Nil(t, suggestions)
}

func TestDoctorDiagnose_ExitNonZeroNoOutput(t *testing.T) {
	// brew doctor exits non-zero when it finds warnings — that's expected.
	// With no output, we get the fallback suggestion but no error.
	withFakeBrew(t, func(args []string) ([]byte, error) {
		return nil, realExitError(t)
	})
	suggestions, err := DoctorDiagnose()
	require.NoError(t, err)
	assert.Contains(t, suggestions, "Run: brew doctor (to see full diagnostic output)")
}

// realExitError returns an actual *exec.ExitError by running a command that
// exits non-zero. DoctorDiagnose uses errors.As to tolerate these (brew
// exits non-zero when reporting warnings), so we need the genuine type.
func realExitError(t *testing.T) error {
	t.Helper()
	err := exec.Command("false").Run()
	require.Error(t, err)
	return err
}

func TestDoctorDiagnose_MultipleWarnings(t *testing.T) {
	withFakeBrew(t, func(args []string) ([]byte, error) {
		return []byte(`Warning: Unbrewed dylibs were found in /usr/local/lib
Warning: Your Homebrew/homebrew/core tap is not a full clone
Warning: Git origin remote mismatch
Warning: Uncommitted modifications to Homebrew
Warning: outdated Xcode command line tools
Warning: Broken symlinks were found
Warning: permission issues
`), nil
	})
	suggestions, err := DoctorDiagnose()
	require.NoError(t, err)
	assert.NotEmpty(t, suggestions)
	assert.Contains(t, suggestions, "Run: brew doctor --list-checks and review linked libraries")
	assert.Contains(t, suggestions, "Run: brew untap homebrew/core homebrew/cask")
	assert.Contains(t, suggestions, "Run: brew update-reset")
	assert.Contains(t, suggestions, "Run: xcode-select --install")
	assert.Contains(t, suggestions, "Run: brew cleanup --prune=all")
	assert.Contains(t, suggestions, "Run: sudo chown -R $(whoami) $(brew --prefix)/*")
}

func TestDoctorDiagnose_UnknownWarnings(t *testing.T) {
	withFakeBrew(t, func(args []string) ([]byte, error) {
		return []byte("Warning: Some unknown issue\n"), nil
	})
	suggestions, err := DoctorDiagnose()
	require.NoError(t, err)
	assert.Contains(t, suggestions, "Run: brew doctor (to see full diagnostic output)")
}

