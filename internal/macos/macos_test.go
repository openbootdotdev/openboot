package macos

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExpandHome_WithTilde(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	result := expandHome("~/Documents/test.txt")
	expected := filepath.Join(tmpHome, "Documents/test.txt")
	assert.Equal(t, expected, result)
}

func TestExpandHome_WithoutTilde(t *testing.T) {
	path := "/absolute/path/test.txt"
	result := expandHome(path)
	assert.Equal(t, path, result)
}

func TestExpandHome_OnlyTilde(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	result := expandHome("~/")
	expected := filepath.Join(tmpHome, "")
	assert.Equal(t, expected, result)
}

func TestExpandHome_RelativePath(t *testing.T) {
	path := "relative/path/test.txt"
	result := expandHome(path)
	assert.Equal(t, path, result)
}

func TestConfigure_DryRun(t *testing.T) {
	prefs := []Preference{
		{Domain: "com.apple.finder", Key: "ShowPathbar", Type: "bool", Value: "true", Desc: "Test pref"},
	}

	err := Configure(prefs, true)
	assert.NoError(t, err)
}

func TestConfigure_EmptyPreferences(t *testing.T) {
	err := Configure([]Preference{}, false)
	assert.NoError(t, err)
}

func TestConfigure_DryRunMultiple(t *testing.T) {
	prefs := []Preference{
		{Domain: "NSGlobalDomain", Key: "KeyRepeat", Type: "int", Value: "2", Desc: "Fast key repeat"},
		{Domain: "com.apple.finder", Key: "ShowPathbar", Type: "bool", Value: "true", Desc: "Show path bar"},
		{Domain: "com.apple.dock", Key: "tilesize", Type: "int", Value: "48", Desc: "Dock size"},
	}

	err := Configure(prefs, true)
	assert.NoError(t, err)
}

func TestConfigure_ExpandsHomeInValues(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	prefs := []Preference{
		{Domain: "com.apple.screencapture", Key: "location", Type: "string", Value: "~/Screenshots", Desc: "Screenshot dir"},
	}

	err := Configure(prefs, true)
	assert.NoError(t, err)
}

func TestConfigure_DifferentTypes(t *testing.T) {
	prefs := []Preference{
		{Domain: "test", Key: "bool_key", Type: "bool", Value: "true", Desc: "Bool test"},
		{Domain: "test", Key: "int_key", Type: "int", Value: "42", Desc: "Int test"},
		{Domain: "test", Key: "float_key", Type: "float", Value: "3.14", Desc: "Float test"},
		{Domain: "test", Key: "string_key", Type: "string", Value: "test", Desc: "String test"},
	}

	err := Configure(prefs, true)
	assert.NoError(t, err)
}

func TestDefaultPreferences_NotEmpty(t *testing.T) {
	assert.Greater(t, len(DefaultPreferences), 0)
}

func TestDefaultPreferences_HasRequiredFields(t *testing.T) {
	for _, pref := range DefaultPreferences {
		assert.NotEmpty(t, pref.Domain, "Domain should not be empty")
		assert.NotEmpty(t, pref.Key, "Key should not be empty")
		assert.NotEmpty(t, pref.Type, "Type should not be empty")
		assert.NotEmpty(t, pref.Desc, "Description should not be empty")
	}
}

func TestDefaultPreferences_ValidTypes(t *testing.T) {
	validTypes := map[string]bool{
		"bool":   true,
		"int":    true,
		"float":  true,
		"string": true,
	}

	for _, pref := range DefaultPreferences {
		assert.True(t, validTypes[pref.Type], "Type %s is not valid", pref.Type)
	}
}

func TestCreateScreenshotsDir_DryRun(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	err := CreateScreenshotsDir(true)
	assert.NoError(t, err)

	dir := filepath.Join(tmpHome, "Screenshots")
	_, err = os.Stat(dir)
	assert.True(t, os.IsNotExist(err))
}

func TestCreateScreenshotsDir_Creates(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	err := CreateScreenshotsDir(false)
	assert.NoError(t, err)

	dir := filepath.Join(tmpHome, "Screenshots")
	info, err := os.Stat(dir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestCreateScreenshotsDir_AlreadyExists(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	dir := filepath.Join(tmpHome, "Screenshots")
	err := os.MkdirAll(dir, 0755)
	require.NoError(t, err)

	err = CreateScreenshotsDir(false)
	assert.NoError(t, err)
}

func TestRestartAffectedApps_DryRun(t *testing.T) {
	err := RestartAffectedApps(true)
	assert.NoError(t, err)
}

func TestRestartAffectedApps_NoDryRun(t *testing.T) {
	err := RestartAffectedApps(false)
	assert.NoError(t, err)
}

func TestDefaultPreferences_FinderPrefs(t *testing.T) {
	finderPrefs := []Preference{}
	for _, p := range DefaultPreferences {
		if p.Domain == "com.apple.finder" {
			finderPrefs = append(finderPrefs, p)
		}
	}
	assert.Greater(t, len(finderPrefs), 0, "Should have Finder preferences")
}

func TestDefaultPreferences_DockPrefs(t *testing.T) {
	dockPrefs := []Preference{}
	for _, p := range DefaultPreferences {
		if p.Domain == "com.apple.dock" {
			dockPrefs = append(dockPrefs, p)
		}
	}
	assert.Greater(t, len(dockPrefs), 0, "Should have Dock preferences")
}

func TestDefaultPreferences_ScreencapturePrefs(t *testing.T) {
	screencapturePrefs := []Preference{}
	for _, p := range DefaultPreferences {
		if p.Domain == "com.apple.screencapture" {
			screencapturePrefs = append(screencapturePrefs, p)
		}
	}
	assert.Greater(t, len(screencapturePrefs), 0, "Should have Screencapture preferences")
}
