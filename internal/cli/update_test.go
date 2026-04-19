package cli

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resetUpdateFlags restores the package-level update flag vars to their zero
// values. Call via t.Cleanup to isolate each test case.
func resetUpdateFlags(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		updateVersion = ""
		updateRollback = false
		updateListBackups = false
		updateDryRun = false
	})
}

// stubSeams swaps the update-command test seams, returning a restore func
// for t.Cleanup. All args are optional — pass nil to keep the real impl.
func stubSeams(t *testing.T,
	isHomebrew func() bool,
	getLatest func() (string, error),
	download func(target, current string) error,
	rollback func() error,
	listBackups func() ([]string, error),
	getBackupDir func() (string, error),
) {
	t.Helper()
	origHB := updateIsHomebrewInstall
	origLatest := updateGetLatestVersion
	origDL := updateDownloadAndReplace
	origRB := updateRollbackFn
	origLB := updateListBackupsFn
	origBD := updateGetBackupDir
	if isHomebrew != nil {
		updateIsHomebrewInstall = isHomebrew
	}
	if getLatest != nil {
		updateGetLatestVersion = getLatest
	}
	if download != nil {
		updateDownloadAndReplace = download
	}
	if rollback != nil {
		updateRollbackFn = rollback
	}
	if listBackups != nil {
		updateListBackupsFn = listBackups
	}
	if getBackupDir != nil {
		updateGetBackupDir = getBackupDir
	}
	t.Cleanup(func() {
		updateIsHomebrewInstall = origHB
		updateGetLatestVersion = origLatest
		updateDownloadAndReplace = origDL
		updateRollbackFn = origRB
		updateListBackupsFn = origLB
		updateGetBackupDir = origBD
	})
}

// ---------------------------------------------------------------------------
// Mutex-flag validation
// ---------------------------------------------------------------------------

func TestRunUpdateCmd_MutexFlags(t *testing.T) {
	cases := []struct {
		name         string
		version      string
		rollback     bool
		listBackups  bool
		wantErrSub   string
		expectRouted bool
	}{
		{name: "version+rollback rejected", version: "1.2.3", rollback: true, wantErrSub: "mutually exclusive"},
		{name: "rollback+list rejected", rollback: true, listBackups: true, wantErrSub: "mutually exclusive"},
		{name: "version+list rejected", version: "1.2.3", listBackups: true, wantErrSub: "mutually exclusive"},
		{name: "all three rejected", version: "1.2.3", rollback: true, listBackups: true, wantErrSub: "mutually exclusive"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resetUpdateFlags(t)
			updateVersion = tc.version
			updateRollback = tc.rollback
			updateListBackups = tc.listBackups

			err := runUpdateCmd(nil, nil)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErrSub)
		})
	}
}

// ---------------------------------------------------------------------------
// Homebrew refusal
// ---------------------------------------------------------------------------

func TestRunUpdate_HomebrewRefusesPin(t *testing.T) {
	resetUpdateFlags(t)
	updateVersion = "1.2.3"
	stubSeams(t, func() bool { return true }, nil, nil, nil, nil, nil)

	err := runUpdateCmd(nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Homebrew")
}

func TestRunUpdate_HomebrewRefusesRollback(t *testing.T) {
	resetUpdateFlags(t)
	updateRollback = true
	stubSeams(t, func() bool { return true }, nil, nil, nil, nil, nil)

	err := runUpdateCmd(nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Homebrew")
}

func TestRunUpdate_HomebrewRefusesLatest(t *testing.T) {
	resetUpdateFlags(t)
	stubSeams(t, func() bool { return true }, nil, nil, nil, nil, nil)

	err := runUpdateCmd(nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "brew upgrade openboot")
}

func TestRunUpdate_HomebrewAllowsListBackups(t *testing.T) {
	// --list-backups is read-only and should be allowed on Homebrew too.
	resetUpdateFlags(t)
	updateListBackups = true
	stubSeams(t, func() bool { return true }, nil, nil, nil,
		func() ([]string, error) { return nil, nil },
		func() (string, error) { return "/tmp/backup", nil },
	)

	err := runUpdateCmd(nil, nil)
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// Semver validation
// ---------------------------------------------------------------------------

func TestRunUpdate_InvalidSemverRejected(t *testing.T) {
	resetUpdateFlags(t)
	updateVersion = "not-a-version"
	stubSeams(t, func() bool { return false }, nil, nil, nil, nil, nil)

	err := runUpdateCmd(nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid version")
}

// ---------------------------------------------------------------------------
// Dry-run paths
// ---------------------------------------------------------------------------

func TestRunUpdate_DryRunPin_DoesNotDownload(t *testing.T) {
	resetUpdateFlags(t)
	updateVersion = "1.2.3"
	updateDryRun = true
	called := false
	stubSeams(t,
		func() bool { return false },
		nil,
		func(target, current string) error { called = true; return nil },
		nil, nil, nil,
	)

	err := runUpdateCmd(nil, nil)
	require.NoError(t, err)
	assert.False(t, called, "dry-run must not call DownloadAndReplace")
}

func TestRunUpdate_DryRunLatest_DoesNotNetwork(t *testing.T) {
	resetUpdateFlags(t)
	updateDryRun = true
	latestCalled := false
	stubSeams(t,
		func() bool { return false },
		func() (string, error) { latestCalled = true; return "1.0.0", nil },
		nil, nil, nil, nil,
	)

	err := runUpdateCmd(nil, nil)
	require.NoError(t, err)
	assert.False(t, latestCalled, "dry-run must not call GetLatestVersion")
}

func TestRunUpdate_DryRunRollback_ReportsBackup(t *testing.T) {
	resetUpdateFlags(t)
	updateRollback = true
	updateDryRun = true
	rollbackCalled := false
	stubSeams(t,
		func() bool { return false },
		nil, nil,
		func() error { rollbackCalled = true; return nil },
		func() ([]string, error) { return []string{"openboot-1.0.0-20260101T000000Z"}, nil },
		nil,
	)

	err := runUpdateCmd(nil, nil)
	require.NoError(t, err)
	assert.False(t, rollbackCalled, "dry-run must not actually roll back")
}

func TestRunUpdate_DryRunRollback_NoBackups(t *testing.T) {
	resetUpdateFlags(t)
	updateRollback = true
	updateDryRun = true
	stubSeams(t,
		func() bool { return false },
		nil, nil, nil,
		func() ([]string, error) { return nil, nil },
		nil,
	)

	err := runUpdateCmd(nil, nil)
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// Delegation to updater functions
// ---------------------------------------------------------------------------

func TestRunUpdate_PinDelegatesToDownload(t *testing.T) {
	resetUpdateFlags(t)
	updateVersion = "1.2.3"
	var gotTarget, gotCurrent string
	stubSeams(t,
		func() bool { return false },
		nil,
		func(target, current string) error {
			gotTarget = target
			gotCurrent = current
			return nil
		},
		nil, nil, nil,
	)
	// Seed the package-level `version` so runPinnedUpgrade forwards it.
	origVersion := version
	version = "0.9.0"
	t.Cleanup(func() { version = origVersion })

	err := runUpdateCmd(nil, nil)
	require.NoError(t, err)
	assert.Equal(t, "1.2.3", gotTarget)
	assert.Equal(t, "0.9.0", gotCurrent, "current running version must be forwarded for backup labeling")
}

func TestRunUpdate_LatestDelegatesToDownload(t *testing.T) {
	resetUpdateFlags(t)
	var gotTarget string
	stubSeams(t,
		func() bool { return false },
		func() (string, error) { return "2.0.0", nil },
		func(target, current string) error { gotTarget = target; return nil },
		nil, nil, nil,
	)

	err := runUpdateCmd(nil, nil)
	require.NoError(t, err)
	assert.Equal(t, "2.0.0", gotTarget)
}

func TestRunUpdate_LatestSurfacesLookupError(t *testing.T) {
	resetUpdateFlags(t)
	stubSeams(t,
		func() bool { return false },
		func() (string, error) { return "", errors.New("github unreachable") },
		nil, nil, nil, nil,
	)

	err := runUpdateCmd(nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "github unreachable")
}

func TestRunUpdate_DownloadErrorWrapped(t *testing.T) {
	resetUpdateFlags(t)
	updateVersion = "1.2.3"
	stubSeams(t,
		func() bool { return false },
		nil,
		func(target, current string) error { return errors.New("checksum mismatch") },
		nil, nil, nil,
	)

	err := runUpdateCmd(nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "update failed")
	assert.Contains(t, err.Error(), "checksum mismatch")
}

func TestRunUpdate_RollbackDelegates(t *testing.T) {
	resetUpdateFlags(t)
	updateRollback = true
	called := false
	stubSeams(t,
		func() bool { return false },
		nil, nil,
		func() error { called = true; return nil },
		nil, nil,
	)

	err := runUpdateCmd(nil, nil)
	require.NoError(t, err)
	assert.True(t, called)
}

func TestRunUpdate_RollbackErrorWrapped(t *testing.T) {
	resetUpdateFlags(t)
	updateRollback = true
	stubSeams(t,
		func() bool { return false },
		nil, nil,
		func() error { return errors.New("no backups found") },
		nil, nil,
	)

	err := runUpdateCmd(nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rollback")
	assert.Contains(t, err.Error(), "no backups found")
}

// ---------------------------------------------------------------------------
// runListBackups
// ---------------------------------------------------------------------------

func TestRunListBackups_EmptyDir(t *testing.T) {
	resetUpdateFlags(t)
	updateListBackups = true
	stubSeams(t, nil, nil, nil, nil,
		func() ([]string, error) { return nil, nil },
		func() (string, error) { return "/tmp/backup", nil },
	)

	err := runUpdateCmd(nil, nil)
	require.NoError(t, err)
}

func TestRunListBackups_WithEntries(t *testing.T) {
	resetUpdateFlags(t)
	updateListBackups = true
	stubSeams(t, nil, nil, nil, nil,
		func() ([]string, error) {
			return []string{
				"openboot-1.0.0-20260101T000000Z",
				"openboot-0.9.0-20251201T000000Z",
			}, nil
		},
		func() (string, error) { return "/tmp/backup", nil },
	)

	err := runUpdateCmd(nil, nil)
	require.NoError(t, err)
}

func TestRunListBackups_ListError(t *testing.T) {
	resetUpdateFlags(t)
	updateListBackups = true
	stubSeams(t, nil, nil, nil, nil,
		func() ([]string, error) { return nil, errors.New("permission denied") },
		func() (string, error) { return "/tmp/backup", nil },
	)

	err := runUpdateCmd(nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "list backups")
	assert.Contains(t, err.Error(), "permission denied")
}

func TestRunListBackups_BackupDirError(t *testing.T) {
	resetUpdateFlags(t)
	updateListBackups = true
	stubSeams(t, nil, nil, nil, nil, nil,
		func() (string, error) { return "", errors.New("home dir unavailable") },
	)

	err := runUpdateCmd(nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolve backup dir")
}
