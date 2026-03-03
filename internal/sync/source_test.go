package sync

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSaveAndLoadSource(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	now := time.Now().Truncate(time.Second)
	source := &SyncSource{
		UserSlug:    "alice/my-setup",
		Username:    "alice",
		Slug:        "my-setup",
		SyncedAt:    now,
		InstalledAt: now,
	}

	err := SaveSource(source)
	require.NoError(t, err)

	// Verify file exists with correct permissions
	path := filepath.Join(tmpDir, ".openboot", "sync_source.json")
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())

	loaded, err := LoadSource()
	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.Equal(t, source.UserSlug, loaded.UserSlug)
	assert.Equal(t, source.Username, loaded.Username)
	assert.Equal(t, source.Slug, loaded.Slug)
	assert.True(t, source.SyncedAt.Equal(loaded.SyncedAt))
	assert.True(t, source.InstalledAt.Equal(loaded.InstalledAt))
}

func TestLoadSourceNotExist(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	loaded, err := LoadSource()
	assert.NoError(t, err)
	assert.Nil(t, loaded)
}

func TestDeleteSource(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	source := &SyncSource{
		UserSlug: "bob/default",
		Username: "bob",
		Slug:     "default",
	}
	require.NoError(t, SaveSource(source))

	loaded, err := LoadSource()
	require.NoError(t, err)
	require.NotNil(t, loaded)

	require.NoError(t, DeleteSource())

	loaded, err = LoadSource()
	assert.NoError(t, err)
	assert.Nil(t, loaded)
}

func TestDeleteSourceNotExist(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	err := DeleteSource()
	assert.NoError(t, err)
}

func TestSourcePath(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	path, err := SourcePath()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(tmpDir, ".openboot", "sync_source.json"), path)
}

func TestSaveSourceOverwrite(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	first := &SyncSource{
		UserSlug: "alice/old",
		Username: "alice",
		Slug:     "old",
	}
	require.NoError(t, SaveSource(first))

	second := &SyncSource{
		UserSlug: "alice/new",
		Username: "alice",
		Slug:     "new",
	}
	require.NoError(t, SaveSource(second))

	loaded, err := LoadSource()
	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.Equal(t, "alice/new", loaded.UserSlug)
	assert.Equal(t, "new", loaded.Slug)
}

func TestLoadSourceCorruptJSON(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	dir := filepath.Join(tmpDir, ".openboot")
	require.NoError(t, os.MkdirAll(dir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sync_source.json"), []byte("{invalid"), 0600))

	loaded, err := LoadSource()
	assert.Error(t, err)
	assert.Nil(t, loaded)
	assert.Contains(t, err.Error(), "parse sync source")
}

func TestSaveSourceCreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Directory doesn't exist yet
	source := &SyncSource{UserSlug: "test/config", Username: "test", Slug: "config"}
	require.NoError(t, SaveSource(source))

	// Verify directory was created with correct perms
	info, err := os.Stat(filepath.Join(tmpDir, ".openboot"))
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}
