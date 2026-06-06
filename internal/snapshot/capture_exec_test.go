package snapshot

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// isBrewInstalled
// ---------------------------------------------------------------------------

// TestIsBrewInstalled_ReturnsBool verifies the function returns a bool without panicking.
func TestIsBrewInstalled_ReturnsBool(t *testing.T) {
	result := isBrewInstalled()
	// We don't know whether brew is installed in the CI environment, but the
	// function must not panic and must return a bool.
	_ = result
}

// ---------------------------------------------------------------------------
// captureBrewList — when brew is absent, returns empty list
// ---------------------------------------------------------------------------

// TestCaptureBrewList_NoPanic verifies the function does not panic regardless of brew availability.
func TestCaptureBrewList_NoPanic(t *testing.T) {
	list, err := captureBrewList("leaves")
	// When brew is absent this returns ([]string{}, nil).
	// When brew is present and succeeds it returns (names, nil).
	// When brew fails it returns ([]string{}, err).
	// In all cases the function must not panic.
	_ = list
	_ = err
}

// ---------------------------------------------------------------------------
// CaptureFormulae / CaptureCasks / CaptureTaps
// ---------------------------------------------------------------------------

// TestCaptureFormulae_NoPanic ensures CaptureFormulae does not panic.
func TestCaptureFormulae_NoPanic(t *testing.T) {
	formulae, err := CaptureFormulae()
	// Returns empty slice when brew is absent — never an error.
	if !isBrewInstalled() {
		require.NoError(t, err)
		assert.NotNil(t, formulae)
	} else {
		// With brew installed the list may or may not be empty but must not panic.
		_ = formulae
		_ = err
	}
}

// TestCaptureCasks_NoPanic ensures CaptureCasks does not panic.
func TestCaptureCasks_NoPanic(t *testing.T) {
	casks, err := CaptureCasks()
	if !isBrewInstalled() {
		require.NoError(t, err)
		assert.NotNil(t, casks)
	} else {
		_ = casks
		_ = err
	}
}

// TestCaptureTaps_NoPanic ensures CaptureTaps does not panic.
func TestCaptureTaps_NoPanic(t *testing.T) {
	taps, err := CaptureTaps()
	if !isBrewInstalled() {
		require.NoError(t, err)
		assert.NotNil(t, taps)
	} else {
		_ = taps
		_ = err
	}
}

// ---------------------------------------------------------------------------
// CaptureNpm
// ---------------------------------------------------------------------------

// TestCaptureNpm_NoPanic ensures CaptureNpm does not panic.
func TestCaptureNpm_NoPanic(t *testing.T) {
	packages, err := CaptureNpm()
	// On a machine without npm this returns ([]string{}, nil).
	// On a machine with npm this returns installed globals.
	// In both cases: must not panic and err must be nil (best-effort capture).
	require.NoError(t, err)
	assert.NotNil(t, packages)
}

// TestCaptureBun_NoPanic ensures CaptureBun does not panic. Mirrors CaptureNpm:
// returns ([]string{}, nil) when bun is absent, installed globals otherwise.
func TestCaptureBun_NoPanic(t *testing.T) {
	packages, err := CaptureBun()
	require.NoError(t, err)
	assert.NotNil(t, packages)
}

// ---------------------------------------------------------------------------
// CaptureMacOSPrefs
// ---------------------------------------------------------------------------

// TestCaptureMacOSPrefs_NoPanic verifies CaptureMacOSPrefs does not panic.
func TestCaptureMacOSPrefs_NoPanic(t *testing.T) {
	prefs, err := CaptureMacOSPrefs()
	// On macOS with defaults(1) available this may return real prefs.
	// In both cases must not return an error and must not panic.
	require.NoError(t, err)
	assert.NotNil(t, prefs)
}

// ---------------------------------------------------------------------------
// CaptureGit
// ---------------------------------------------------------------------------

// TestCaptureGit_ReturnsSnap verifies CaptureGit returns a non-nil snapshot.
func TestCaptureGit_ReturnsSnap(t *testing.T) {
	snap, err := CaptureGit()
	require.NoError(t, err)
	require.NotNil(t, snap)
	// UserName and UserEmail may be empty strings on a machine with no git config —
	// that is valid. The function must not error.
}

// TestCaptureGit_FieldsAreStrings verifies the returned fields are strings (not nil).
func TestCaptureGit_FieldsAreStrings(t *testing.T) {
	snap, err := CaptureGit()
	require.NoError(t, err)

	// These are plain string fields — no nil check needed, but we assert they
	// don't contain unexpected whitespace from mis-trimmed output.
	assert.NotContains(t, snap.UserName, "\n")
	assert.NotContains(t, snap.UserEmail, "\n")
}

// ---------------------------------------------------------------------------
// CaptureDevTools
// ---------------------------------------------------------------------------

// TestCaptureDevTools_ReturnsSlice verifies CaptureDevTools returns a slice (possibly empty).
func TestCaptureDevTools_ReturnsSlice(t *testing.T) {
	tools, err := CaptureDevTools()
	require.NoError(t, err)
	assert.NotNil(t, tools)
}

// TestCaptureDevTools_VersionsNotEmpty verifies any captured tool has a non-empty name.
func TestCaptureDevTools_VersionsNotEmpty(t *testing.T) {
	tools, err := CaptureDevTools()
	require.NoError(t, err)
	for _, dt := range tools {
		assert.NotEmpty(t, dt.Name, "captured DevTool must have a name")
	}
}

// ---------------------------------------------------------------------------
// CaptureDotfiles — isolation scenarios
// ---------------------------------------------------------------------------

// TestCaptureDotfiles_EmptyHome verifies empty snapshot when HOME has no .dotfiles.
func TestCaptureDotfiles_EmptyHome(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	snap, err := CaptureDotfiles()
	require.NoError(t, err)
	require.NotNil(t, snap)
	// No .dotfiles directory → RepoURL should be empty.
	assert.Empty(t, snap.RepoURL)
}

// TestCaptureDotfiles_DotfilesDirMissingGitSubdir verifies empty snapshot when .dotfiles has no .git.
func TestCaptureDotfiles_DotfilesDirMissingGitSubdir(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Create .dotfiles directory without a .git subdirectory.
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, ".dotfiles"), 0755))

	snap, err := CaptureDotfiles()
	require.NoError(t, err)
	require.NotNil(t, snap)
	// No .git → RepoURL should be empty.
	assert.Empty(t, snap.RepoURL)
}

// ---------------------------------------------------------------------------
// Capture — top-level function
// ---------------------------------------------------------------------------

// TestCapture_ReturnsSnapshotWithoutError verifies the top-level Capture function
// returns a complete Snapshot and does not error in a normal environment.
func TestCapture_ReturnsSnapshotWithoutError(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	snap, err := Capture()
	require.NoError(t, err)
	require.NotNil(t, snap)
	assert.Equal(t, 1, snap.Version)
	assert.NotEmpty(t, snap.Hostname)
	// Packages may be empty in CI but must not be nil.
	assert.NotNil(t, snap.Packages.Formulae)
	assert.NotNil(t, snap.Packages.Casks)
	assert.NotNil(t, snap.Packages.Npm)
	assert.NotNil(t, snap.DevTools)
}

// TestCapture_CapturedAtIsRecent verifies CapturedAt is set to a recent time.
func TestCapture_CapturedAtIsRecent(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	snap, err := Capture()
	require.NoError(t, err)

	assert.False(t, snap.CapturedAt.IsZero(), "CapturedAt must be set")
}

// TestCapture_HealthRecordsFailedSteps verifies that Capture() propagates step
// failures into Health rather than aborting the whole capture.
func TestCapture_HealthRecordsFailedSteps(t *testing.T) {
	orig := captureSteps
	steps := make([]captureStep, len(orig))
	copy(steps, orig)
	// Inject a failure into the first step (Homebrew Formulae).
	steps[0] = captureStep{
		name:    "Homebrew Formulae",
		capture: func(r *CaptureResults) error { return fmt.Errorf("injected failure") },
		count:   func(r *CaptureResults) int { return len(r.Formulae) },
	}
	captureSteps = steps
	t.Cleanup(func() { captureSteps = orig })

	// Call the public Capture() entry point — not CaptureWithProgress directly —
	// so the test breaks if the two ever diverge.
	snap, err := Capture()
	require.NoError(t, err)
	require.NotNil(t, snap)

	assert.True(t, snap.Health.Partial, "Health.Partial must be true when a step fails")
	assert.Contains(t, snap.Health.FailedSteps, "Homebrew Formulae")
}
