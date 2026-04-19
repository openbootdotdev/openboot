// NOTE: tests in this file must NOT use t.Parallel() due to the shared
// package-level backupDirOverride variable.
package updater

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Semver validation ---

func TestValidateSemver(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"plain semver", "0.25.0", false},
		{"with v prefix", "v0.25.0", false},
		{"multi-digit components", "12.345.6789", false},
		{"empty", "", true},
		{"not semver word", "latest", true},
		{"incomplete two-part", "1.2", true},
		{"four parts", "1.2.3.4", true},
		{"non-numeric major", "a.1.1", true},
		{"prerelease suffix", "1.2.3-rc1", true},
		{"leading space", " 1.2.3", true},
		{"trailing newline", "1.2.3\n", true},
		{"double v prefix", "vv1.2.3", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSemver(tt.input)
			if tt.wantErr {
				assert.Error(t, err, "ValidateSemver(%q) expected error", tt.input)
			} else {
				assert.NoError(t, err, "ValidateSemver(%q) unexpected error", tt.input)
			}
		})
	}
}

// --- URL construction ---

func TestChecksumsURL(t *testing.T) {
	assert.Equal(t,
		"https://github.com/openbootdotdev/openboot/releases/latest/download/checksums.txt",
		checksumsURL(""),
		"empty version should use /latest/",
	)
	assert.Equal(t,
		"https://github.com/openbootdotdev/openboot/releases/download/v0.25.0/checksums.txt",
		checksumsURL("0.25.0"),
	)
	assert.Equal(t,
		"https://github.com/openbootdotdev/openboot/releases/download/v0.25.0/checksums.txt",
		checksumsURL("v0.25.0"),
		"leading v should be normalized",
	)
}

func TestBinaryURL(t *testing.T) {
	assert.Equal(t,
		"https://github.com/openbootdotdev/openboot/releases/latest/download/openboot-darwin-arm64",
		binaryURL("", "openboot-darwin-arm64"),
	)
	assert.Equal(t,
		"https://github.com/openbootdotdev/openboot/releases/download/v0.25.0/openboot-darwin-arm64",
		binaryURL("0.25.0", "openboot-darwin-arm64"),
	)
	assert.Equal(t,
		"https://github.com/openbootdotdev/openboot/releases/download/v0.25.0/openboot-darwin-arm64",
		binaryURL("v0.25.0", "openboot-darwin-arm64"),
	)
}

// --- Backup directory / creation / pruning ---

func TestGetBackupDir_DefaultsToHome(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	SetBackupDirForTesting("")
	dir, err := GetBackupDir()
	require.NoError(t, err)
	assert.Contains(t, dir, ".openboot")
	assert.Contains(t, dir, "backup")
}

func TestGetBackupDir_Override(t *testing.T) {
	tmp := t.TempDir()
	SetBackupDirForTesting(tmp)
	defer SetBackupDirForTesting("")
	dir, err := GetBackupDir()
	require.NoError(t, err)
	assert.Equal(t, tmp, dir)
}

func TestBackupCurrentBinary_CreatesFile(t *testing.T) {
	tmp := t.TempDir()
	backupDir := filepath.Join(tmp, "backup")
	SetBackupDirForTesting(backupDir)
	defer SetBackupDirForTesting("")

	bin := filepath.Join(tmp, "openboot")
	require.NoError(t, os.WriteFile(bin, []byte("fake binary"), 0755))

	require.NoError(t, backupCurrentBinary(bin, "1.2.3"))

	entries, err := os.ReadDir(backupDir)
	require.NoError(t, err)
	require.Len(t, entries, 1, "expected one backup file")
	name := entries[0].Name()
	assert.Contains(t, name, "openboot-1.2.3-")

	// Contents must match the source.
	data, err := os.ReadFile(filepath.Join(backupDir, name))
	require.NoError(t, err)
	assert.Equal(t, []byte("fake binary"), data)

	// File mode must be 0755 so the restored binary is executable.
	info, err := os.Stat(filepath.Join(backupDir, name))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0755), info.Mode().Perm())
}

func TestBackupCurrentBinary_UnknownVersionWhenBlank(t *testing.T) {
	tmp := t.TempDir()
	backupDir := filepath.Join(tmp, "backup")
	SetBackupDirForTesting(backupDir)
	defer SetBackupDirForTesting("")

	bin := filepath.Join(tmp, "openboot")
	require.NoError(t, os.WriteFile(bin, []byte("x"), 0755))
	require.NoError(t, backupCurrentBinary(bin, ""))

	entries, err := os.ReadDir(backupDir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Contains(t, entries[0].Name(), "openboot-unknown-")
}

// Writes n backup files with staggered, deterministic mtimes so prune order
// is unambiguous. Returns names in oldest→newest order.
func seedBackups(t *testing.T, dir string, n int) []string {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0700))
	names := make([]string, n)
	base := time.Now().Add(-time.Duration(n) * time.Hour)
	for i := 0; i < n; i++ {
		name := filepath.Join(dir, "openboot-1.0.0-"+time.Now().Format("20060102T150405Z")+"-"+itoa(i))
		require.NoError(t, os.WriteFile(name, []byte("v"+itoa(i)), 0755))
		mt := base.Add(time.Duration(i) * time.Hour)
		require.NoError(t, os.Chtimes(name, mt, mt))
		names[i] = name
	}
	return names
}

func itoa(i int) string {
	const digits = "0123456789"
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = digits[i%10]
		i /= 10
	}
	return string(buf[pos:])
}

func TestPruneBackups_KeepsNewest(t *testing.T) {
	tmp := t.TempDir()
	names := seedBackups(t, tmp, 7)

	require.NoError(t, pruneBackups(tmp, 5))

	entries, err := os.ReadDir(tmp)
	require.NoError(t, err)
	require.Len(t, entries, 5, "should retain 5 newest")

	// The two oldest should be gone.
	_, err = os.Stat(names[0])
	assert.True(t, os.IsNotExist(err), "oldest backup should be pruned")
	_, err = os.Stat(names[1])
	assert.True(t, os.IsNotExist(err), "second-oldest backup should be pruned")
	// The newest should survive.
	_, err = os.Stat(names[6])
	assert.NoError(t, err, "newest backup should survive prune")
}

func TestPruneBackups_NoOpUnderLimit(t *testing.T) {
	tmp := t.TempDir()
	seedBackups(t, tmp, 3)
	require.NoError(t, pruneBackups(tmp, 5))
	entries, err := os.ReadDir(tmp)
	require.NoError(t, err)
	assert.Len(t, entries, 3)
}

func TestBackupCurrentBinary_PrunesAfterLimit(t *testing.T) {
	tmp := t.TempDir()
	backupDir := filepath.Join(tmp, "backup")
	SetBackupDirForTesting(backupDir)
	defer SetBackupDirForTesting("")

	// Seed one-less-than-limit backups with old mtimes.
	seedBackups(t, backupDir, backupRetention)

	bin := filepath.Join(tmp, "openboot")
	require.NoError(t, os.WriteFile(bin, []byte("new"), 0755))
	require.NoError(t, backupCurrentBinary(bin, "1.0.0"))

	entries, err := os.ReadDir(backupDir)
	require.NoError(t, err)
	assert.Len(t, entries, backupRetention, "should cap at retention limit after a new backup")
}

// --- ListBackups ---

func TestListBackups_EmptyDirReturnsNil(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	SetBackupDirForTesting("")
	names, err := ListBackups()
	require.NoError(t, err)
	assert.Empty(t, names, "missing dir should return empty slice, not error")
}

func TestListBackups_NewestFirst(t *testing.T) {
	tmp := t.TempDir()
	SetBackupDirForTesting(tmp)
	defer SetBackupDirForTesting("")

	names := seedBackups(t, tmp, 3)
	got, err := ListBackups()
	require.NoError(t, err)
	require.Len(t, got, 3)
	// Newest mtime is the last name we seeded.
	assert.Equal(t, filepath.Base(names[2]), got[0], "newest should be first")
	assert.Equal(t, filepath.Base(names[0]), got[2], "oldest should be last")
}

// --- Rollback ---

func TestRollback_NoBackups(t *testing.T) {
	tmp := t.TempDir()
	SetBackupDirForTesting(tmp)
	defer SetBackupDirForTesting("")

	err := Rollback()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no backups")
}

func TestRollback_PicksNewest(t *testing.T) {
	// Create a fake "current binary" in the temp dir so os.Executable() points
	// somewhere we can safely overwrite. We can't override os.Executable()
	// itself, so simulate rollback via the internals: exercise listBackupsSorted
	// and copyFile, which together back Rollback().
	tmp := t.TempDir()
	backupDir := filepath.Join(tmp, "backup")
	require.NoError(t, os.MkdirAll(backupDir, 0700))

	// Seed three backups with escalating mtimes + distinct contents.
	base := time.Now().Add(-3 * time.Hour)
	for i, contents := range []string{"oldest", "middle", "newest"} {
		path := filepath.Join(backupDir, "openboot-1.0.0-"+itoa(i))
		require.NoError(t, os.WriteFile(path, []byte(contents), 0755))
		mt := base.Add(time.Duration(i) * time.Hour)
		require.NoError(t, os.Chtimes(path, mt, mt))
	}

	files, err := listBackupsSorted(backupDir)
	require.NoError(t, err)
	require.Len(t, files, 3)

	// Newest first — "newest" content file should be picked.
	data, err := os.ReadFile(filepath.Join(backupDir, files[0].Name()))
	require.NoError(t, err)
	assert.Equal(t, "newest", string(data))

	// copyFile should produce a 0755 executable clone.
	dst := filepath.Join(tmp, "restored")
	require.NoError(t, copyFile(filepath.Join(backupDir, files[0].Name()), dst, 0755))
	info, err := os.Stat(dst)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0755), info.Mode().Perm())
	got, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, "newest", string(got))
}

func TestRollback_ErrorMessageMentionsBackupDir(t *testing.T) {
	tmp := t.TempDir()
	SetBackupDirForTesting(tmp)
	defer SetBackupDirForTesting("")
	err := Rollback()
	require.Error(t, err)
	assert.Contains(t, err.Error(), tmp, "error should mention the backup directory")
}
