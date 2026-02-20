package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultStatePath(t *testing.T) {
	path := DefaultStatePath()
	assert.NotEmpty(t, path)
	assert.Contains(t, path, ".openboot")
	assert.Contains(t, path, "state.json")
}

func TestLoadState_Fresh(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	state, err := LoadState(statePath)
	require.NoError(t, err)
	assert.NotNil(t, state)
	assert.False(t, state.Dismissed)
	assert.False(t, state.Skipped)
}

func TestLoadState_Existing(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	expected := &ReminderState{Dismissed: true, Skipped: false}
	data, err := json.MarshalIndent(expected, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(statePath, data, 0644))

	state, err := LoadState(statePath)
	require.NoError(t, err)
	assert.NotNil(t, state)
	assert.True(t, state.Dismissed)
	assert.False(t, state.Skipped)
}

func TestLoadState_Corrupted(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	require.NoError(t, os.WriteFile(statePath, []byte("invalid json {"), 0644))

	state, err := LoadState(statePath)
	require.NoError(t, err)
	assert.NotNil(t, state)
	assert.False(t, state.Dismissed)
	assert.False(t, state.Skipped)
}

func TestSaveState_Success(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	state := &ReminderState{Dismissed: true, Skipped: true}
	err := SaveState(statePath, state)
	require.NoError(t, err)

	data, err := os.ReadFile(statePath)
	require.NoError(t, err)

	var loaded ReminderState
	err = json.Unmarshal(data, &loaded)
	require.NoError(t, err)
	assert.True(t, loaded.Dismissed)
	assert.True(t, loaded.Skipped)
}

func TestSaveState_DirectoryCreation(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "nested", "dir", "state.json")

	state := &ReminderState{Dismissed: false, Skipped: true}
	err := SaveState(statePath, state)
	require.NoError(t, err)

	_, err = os.Stat(statePath)
	require.NoError(t, err)

	data, err := os.ReadFile(statePath)
	require.NoError(t, err)

	var loaded ReminderState
	err = json.Unmarshal(data, &loaded)
	require.NoError(t, err)
	assert.False(t, loaded.Dismissed)
	assert.True(t, loaded.Skipped)
}

func TestSaveState_Permissions(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	state := &ReminderState{Dismissed: true, Skipped: false}
	err := SaveState(statePath, state)
	require.NoError(t, err)

	info, err := os.Stat(statePath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0644), info.Mode().Perm())
}

func TestSaveState_Indented(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	state := &ReminderState{Dismissed: true, Skipped: false}
	err := SaveState(statePath, state)
	require.NoError(t, err)

	data, err := os.ReadFile(statePath)
	require.NoError(t, err)

	assert.Contains(t, string(data), "  ")
}

func TestShouldShowReminder_NotDismissed(t *testing.T) {
	state := &ReminderState{Dismissed: false, Skipped: false}
	assert.True(t, ShouldShowReminder(state))
}

func TestShouldShowReminder_Dismissed(t *testing.T) {
	state := &ReminderState{Dismissed: true, Skipped: false}
	assert.False(t, ShouldShowReminder(state))
}

func TestMarkDismissed(t *testing.T) {
	state := &ReminderState{Dismissed: false, Skipped: false}
	MarkDismissed(state)
	assert.True(t, state.Dismissed)
	assert.False(t, state.Skipped)
}

func TestMarkSkipped(t *testing.T) {
	state := &ReminderState{Dismissed: false, Skipped: false}
	MarkSkipped(state)
	assert.False(t, state.Dismissed)
	assert.True(t, state.Skipped)
}

func TestRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	original := &ReminderState{Dismissed: true, Skipped: true}
	err := SaveState(statePath, original)
	require.NoError(t, err)

	loaded, err := LoadState(statePath)
	require.NoError(t, err)
	assert.Equal(t, original.Dismissed, loaded.Dismissed)
	assert.Equal(t, original.Skipped, loaded.Skipped)
}

func TestRoundTrip_DefaultState(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	original := &ReminderState{Dismissed: false, Skipped: false}
	err := SaveState(statePath, original)
	require.NoError(t, err)

	loaded, err := LoadState(statePath)
	require.NoError(t, err)
	assert.False(t, loaded.Dismissed)
	assert.False(t, loaded.Skipped)
}

func TestLoadState_ReadError(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	require.NoError(t, os.WriteFile(statePath, []byte("invalid json {"), 0644))

	require.NoError(t, os.Chmod(tmpDir, 0000))
	t.Cleanup(func() {
		os.Chmod(tmpDir, 0755)
	})

	state, err := LoadState(statePath)
	assert.Error(t, err)
	assert.Nil(t, state)
	assert.Contains(t, err.Error(), "read state")
}

func TestSaveState_AtomicWrite(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	state1 := &ReminderState{Dismissed: true, Skipped: false}
	err := SaveState(statePath, state1)
	require.NoError(t, err)

	state2 := &ReminderState{Dismissed: false, Skipped: true}
	err = SaveState(statePath, state2)
	require.NoError(t, err)

	loaded, err := LoadState(statePath)
	require.NoError(t, err)
	assert.False(t, loaded.Dismissed)
	assert.True(t, loaded.Skipped)

	tmpFile := statePath + ".tmp"
	_, err = os.Stat(tmpFile)
	assert.True(t, os.IsNotExist(err), "temp file should be cleaned up")
}
