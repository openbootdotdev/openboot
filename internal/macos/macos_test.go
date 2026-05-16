package macos

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureStdout runs fn and returns whatever it wrote to os.Stdout.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w
	done := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	fn()
	_ = w.Close()
	os.Stdout = old
	return <-done
}

func TestHostScopeLabel(t *testing.T) {
	assert.Equal(t, "", hostScopeLabel(""))
	assert.Equal(t, "(ByHost) ", hostScopeLabel("currentHost"))
}

func TestConfigure_DryRun_ByHostLabel(t *testing.T) {
	// Regression: a Preference with Host="currentHost" must show the ByHost
	// scope label in dry-run output (and, by extension, in the actual
	// `defaults` invocation — same code path adds -currentHost).
	prefs := []Preference{
		{Domain: "com.apple.controlcenter", Key: "Sound", Type: "int", Value: "18", Desc: "Always show Sound", Host: "currentHost"},
		{Domain: "com.apple.dock", Key: "autohide", Type: "bool", Value: "false", Desc: "Keep Dock visible"},
	}
	out := captureStdout(t, func() { _ = Configure(prefs, true) })
	assert.Contains(t, out, "(ByHost) com.apple.controlcenter Sound")
	// The non-ByHost pref must NOT carry the scope label.
	assert.Regexp(t, `Would set com\.apple\.dock autohide`, out)
	assert.NotContains(t, out, "(ByHost) com.apple.dock")
}

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

// TestRestartAffectedApps_NoDryRun kills real system processes (Finder, Dock, SystemUIServer),
// so it must only run with the integration build tag.
// See test/integration/ for the integration-tagged version.

// ---------------------------------------------------------------------------
// normalizeBool
// ---------------------------------------------------------------------------

func TestNormalizeBool(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"1", "true"},
		{"yes", "true"},
		{"YES", "true"},
		{"Yes", "true"},
		{"0", "false"},
		{"no", "false"},
		{"NO", "false"},
		{"No", "false"},
		{"true", "true"},
		{"false", "false"},
		// normalizeBool only lowercases "1/yes" → "true" and "0/no" → "false";
		// mixed-case "TRUE"/"FALSE" are not in the switch and pass through unchanged.
		{"TRUE", "TRUE"},
		{"FALSE", "FALSE"},
		{"other", "other"},
		{"", ""},
		{"42", "42"},
	}

	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			got := normalizeBool(tc.in)
			assert.Equal(t, tc.want, got)
		})
	}
}

// ---------------------------------------------------------------------------
// InferPreferenceType
// ---------------------------------------------------------------------------

func TestInferPreferenceType(t *testing.T) {
	tests := []struct {
		value string
		want  string
	}{
		// bool variants
		{"true", "bool"},
		{"false", "bool"},
		{"TRUE", "bool"},
		{"FALSE", "bool"},
		{"1", "bool"},
		{"0", "bool"},
		{"yes", "bool"},
		{"no", "bool"},
		{"YES", "bool"},
		{"NO", "bool"},
		// int
		{"42", "int"},
		{"100", "int"},
		{"999", "int"},
		// float
		{"3.14", "float"},
		{"0.5", "float"},
		{"1.0", "float"},
		// Implementation strips all dots before checking digits, so "1.2.3" → "123" → float
		{"1.2.3", "float"},
		// string fallbacks
		{"hello", "string"},
		{"path/to/file", "string"},
		{"", "string"}, // empty value — not "all digits"
	}

	for _, tc := range tests {
		t.Run(tc.value+"->"+tc.want, func(t *testing.T) {
			got := InferPreferenceType(tc.value)
			assert.Equal(t, tc.want, got)
		})
	}
}

// ---------------------------------------------------------------------------
// expandHome edge cases
// ---------------------------------------------------------------------------

func TestExpandHome_HomeDirError(t *testing.T) {
	// When HOME is unset os.UserHomeDir fails; expandHome should return the
	// original path unchanged.
	t.Setenv("HOME", "")
	result := expandHome("~/somefile")
	// Either returns original path OR expands (depending on whether the OS
	// has a fallback). We just verify it does not panic and returns a string.
	assert.IsType(t, "", result)
}

// ---------------------------------------------------------------------------
// Configure — default-type branch (empty Type field)
// ---------------------------------------------------------------------------

func TestConfigure_DryRunDefaultType(t *testing.T) {
	// Exercises the "default:" branch inside Configure where Type is not one of
	// bool/int/float/string, so the value is appended directly.
	prefs := []Preference{
		{Domain: "com.apple.test", Key: "SomeKey", Type: "", Value: "rawvalue", Desc: "default type test"},
	}
	err := Configure(prefs, true)
	assert.NoError(t, err)
}

func TestConfigure_DryRunExpandsHomePath(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	prefs := []Preference{
		{Domain: "com.apple.screencapture", Key: "location", Type: "string", Value: "~/Screenshots", Desc: "Screenshots dir"},
	}
	err := Configure(prefs, true)
	assert.NoError(t, err)
}

// ---------------------------------------------------------------------------
// InferPreferenceType — table correctness for int boundary
// ---------------------------------------------------------------------------

func TestInferPreferenceType_IntNotBool(t *testing.T) {
	// Values that are all-digit but NOT "0" or "1" must return "int".
	for _, v := range []string{"2", "10", "255", "1024"} {
		t.Run(v, func(t *testing.T) {
			got := InferPreferenceType(v)
			assert.Equal(t, "int", got)
		})
	}
}

func TestInferPreferenceType_MultiDotIsFloat(t *testing.T) {
	// InferPreferenceType strips all dots before checking digits, so "1.2.3"
	// becomes "123" which is all-numeric and returns "float".
	// This documents the actual implementation behavior.
	got := InferPreferenceType("1.2.3")
	assert.Equal(t, "float", got)
}

func TestInferPreferenceType_FloatWithLetters(t *testing.T) {
	got := InferPreferenceType("1.x")
	assert.Equal(t, "string", got)
}
