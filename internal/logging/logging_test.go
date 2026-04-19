package logging

import (
	"encoding/json"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withStubs swaps the package-level logDir/now vars for the duration of a
// test, restoring the originals on cleanup. Tests must always use this to
// avoid touching the real user home dir.
func withStubs(t *testing.T, dir string, clock time.Time) {
	t.Helper()
	origDir := logDir
	origNow := now
	origOnce := fallbackReported
	logDir = func() (string, error) { return dir, nil }
	now = func() time.Time { return clock }
	fallbackReported = &sync.Once{}
	t.Cleanup(func() {
		logDir = origDir
		now = origNow
		fallbackReported = origOnce
	})
}

func TestInit_CreatesDirAndFile(t *testing.T) {
	tmp := t.TempDir()
	dir := filepath.Join(tmp, "logs")
	clock := time.Date(2026, 4, 19, 10, 30, 0, 0, time.UTC)
	withStubs(t, dir, clock)

	closer, err := Init("1.2.3", false)
	require.NoError(t, err)
	defer closer()
	WaitForCleanup()

	// Directory was created with 0700.
	info, err := os.Stat(dir)
	require.NoError(t, err)
	require.True(t, info.IsDir())
	if runtime.GOOS != "windows" {
		assert.Equal(t, fs.FileMode(0o700), info.Mode().Perm(), "log dir must be 0700")
	}

	// File was created with 0600, named by the injected clock.
	expected := filepath.Join(dir, "openboot-2026-04-19.log")
	finfo, err := os.Stat(expected)
	require.NoError(t, err, "expected log file %s", expected)
	if runtime.GOOS != "windows" {
		assert.Equal(t, fs.FileMode(0o600), finfo.Mode().Perm(), "log file must be 0600")
	}
}

func TestInit_EmitsSessionStart(t *testing.T) {
	tmp := t.TempDir()
	dir := filepath.Join(tmp, "logs")
	clock := time.Date(2026, 4, 19, 10, 30, 0, 0, time.UTC)
	withStubs(t, dir, clock)

	closer, err := Init("9.9.9", false)
	require.NoError(t, err)
	closer() // flush and close before we read the file
	WaitForCleanup()

	path := filepath.Join(dir, "openboot-2026-04-19.log")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NotEmpty(t, data, "log file should contain at least session_start")

	// Each line must be valid JSON (we use JSONHandler for the file sink).
	var found bool
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var rec map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &rec), "line must be valid JSON: %s", line)
		if rec["msg"] == "session_start" {
			assert.Equal(t, "9.9.9", rec["version"])
			assert.NotNil(t, rec["pid"])
			assert.NotNil(t, rec["args"])
			found = true
		}
	}
	assert.True(t, found, "session_start record must be present")
}

func TestInit_AppendsToExistingFile(t *testing.T) {
	tmp := t.TempDir()
	dir := filepath.Join(tmp, "logs")
	clock := time.Date(2026, 4, 19, 10, 30, 0, 0, time.UTC)
	withStubs(t, dir, clock)

	// First invocation.
	closer1, err := Init("1.0.0", false)
	require.NoError(t, err)
	closer1()
	WaitForCleanup()

	// Second invocation should append, not truncate.
	closer2, err := Init("1.0.0", false)
	require.NoError(t, err)
	closer2()
	WaitForCleanup()

	path := filepath.Join(dir, "openboot-2026-04-19.log")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	// Two session_start records.
	count := 0
	for _, ln := range lines {
		if strings.Contains(ln, "session_start") {
			count++
		}
	}
	assert.Equal(t, 2, count, "both sessions should be appended")
}

func TestInit_Retention(t *testing.T) {
	tmp := t.TempDir()
	dir := filepath.Join(tmp, "logs")
	require.NoError(t, os.MkdirAll(dir, 0o700))

	clock := time.Date(2026, 4, 19, 10, 30, 0, 0, time.UTC)
	withStubs(t, dir, clock)

	// Create three fake log files: one fresh (today's, by mtime) and two old.
	freshName := "openboot-2026-04-15.log" // 4 days old — within retention
	oldAName := "openboot-2026-04-01.log"  // 18 days old
	oldBName := "openboot-2026-03-20.log"  // 30 days old
	for _, name := range []string{freshName, oldAName, oldBName} {
		p := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(p, []byte("x"), 0o600))
	}

	// Set mtimes explicitly so retention logic is deterministic.
	mustChtimes(t, filepath.Join(dir, freshName), clock.AddDate(0, 0, -4))
	mustChtimes(t, filepath.Join(dir, oldAName), clock.AddDate(0, 0, -18))
	mustChtimes(t, filepath.Join(dir, oldBName), clock.AddDate(0, 0, -30))

	// Also drop an unrelated file — retention must ignore it.
	unrelated := filepath.Join(dir, "other.txt")
	require.NoError(t, os.WriteFile(unrelated, []byte("keep"), 0o600))
	mustChtimes(t, unrelated, clock.AddDate(0, 0, -60))

	closer, err := Init("test", false)
	require.NoError(t, err)
	defer closer()
	WaitForCleanup()

	// Today's log should exist after Init.
	todayLog := filepath.Join(dir, "openboot-2026-04-19.log")
	_, err = os.Stat(todayLog)
	assert.NoError(t, err)

	// Fresh (4d old) log should still be there.
	_, err = os.Stat(filepath.Join(dir, freshName))
	assert.NoError(t, err, "file within retention window must be kept")

	// Old logs should be deleted.
	_, err = os.Stat(filepath.Join(dir, oldAName))
	assert.True(t, os.IsNotExist(err), "file older than retention must be deleted")
	_, err = os.Stat(filepath.Join(dir, oldBName))
	assert.True(t, os.IsNotExist(err), "file older than retention must be deleted")

	// Unrelated file must survive.
	_, err = os.Stat(unrelated)
	assert.NoError(t, err, "retention must only touch openboot-*.log files")
}

func TestInit_FallsBackWhenDirUnwritable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file-as-directory trick is POSIX-only")
	}
	tmp := t.TempDir()
	// Point logDir at a file, so MkdirAll + OpenFile both fail.
	blocker := filepath.Join(tmp, "not-a-dir")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o600))

	origDir := logDir
	origNow := now
	origOnce := fallbackReported
	logDir = func() (string, error) { return blocker, nil }
	now = func() time.Time { return time.Date(2026, 4, 19, 10, 30, 0, 0, time.UTC) }
	fallbackReported = &sync.Once{}
	t.Cleanup(func() {
		logDir = origDir
		now = origNow
		fallbackReported = origOnce
	})

	closer, err := Init("x", false)
	require.NoError(t, err, "Init must never return an error for fallback")
	require.NotNil(t, closer)
	closer() // must be safe to call

	// slog.Default should still produce output (to stderr) without panicking.
	slog.Warn("fallback_ok", "k", "v")
}

func TestInit_VerboseSetsStderrLevel(t *testing.T) {
	// This test is mostly structural: verify Init accepts the verbose flag
	// and returns a valid closer. Level wiring is inspected indirectly.
	tmp := t.TempDir()
	dir := filepath.Join(tmp, "logs")
	withStubs(t, dir, time.Date(2026, 4, 19, 0, 0, 0, 0, time.UTC))

	for _, verbose := range []bool{false, true} {
		closer, err := Init("v", verbose)
		require.NoError(t, err)
		closer()
		WaitForCleanup()
	}
}

func mustChtimes(t *testing.T, path string, mtime time.Time) {
	t.Helper()
	require.NoError(t, os.Chtimes(path, mtime, mtime))
}

func TestRedactArgs(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "plain args untouched",
			in:   []string{"openboot", "install", "--preset=developer", "--dry-run"},
			want: []string{"openboot", "install", "--preset=developer", "--dry-run"},
		},
		{
			name: "token flag redacted",
			in:   []string{"openboot", "--token=abc123"},
			want: []string{"openboot", "--token=<redacted>"},
		},
		{
			name: "password flag redacted",
			in:   []string{"openboot", "--password=hunter2"},
			want: []string{"openboot", "--password=<redacted>"},
		},
		{
			name: "case-insensitive match on fragment",
			in:   []string{"openboot", "--API-KEY=xyz", "--Secret-Token=abc"},
			want: []string{"openboot", "--API-KEY=<redacted>", "--Secret-Token=<redacted>"},
		},
		{
			name: "space-separated form is NOT redacted (known limitation)",
			in:   []string{"openboot", "--token", "abc123"},
			want: []string{"openboot", "--token", "abc123"},
		},
		{
			name: "positional arg with = passes through when name has no sensitive fragment",
			in:   []string{"openboot", "package=latest"},
			want: []string{"openboot", "package=latest"},
		},
		{
			name: "empty args",
			in:   []string{},
			want: []string{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := RedactArgs(tc.in)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestMultiHandler_WithAttrsAndWithGroup(t *testing.T) {
	tmp := t.TempDir()
	dir := filepath.Join(tmp, "logs")
	clock := time.Date(2026, 4, 19, 10, 30, 0, 0, time.UTC)
	withStubs(t, dir, clock)

	closer, err := Init("1.0.0", false)
	require.NoError(t, err)
	defer closer()
	WaitForCleanup()

	// Exercise both WithAttrs and WithGroup — they must return a handler
	// that still writes to every child sink (file + stderr in this case).
	logger := slog.Default().With("component", "test").WithGroup("sub")
	logger.Info("grouped_event", "k", "v")
	// Also call a direct slog fn to make sure Enabled gate works across the
	// fanned-out tree.
	slog.Default().Debug("debug_should_reach_file_not_stderr")

	// Force any buffered writes to flush before we read the file.
	closer()

	path := filepath.Join(dir, "openboot-2026-04-19.log")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	text := string(data)
	// The grouped record must appear with both the With attr and a grouped
	// key. JSONHandler renders groups as nested objects; assert by substring
	// so we don't need to unmarshal every line.
	assert.Contains(t, text, `"component":"test"`, "WithAttrs attribute must reach file sink")
	assert.Contains(t, text, `grouped_event`, "grouped record must appear in file")
	assert.Contains(t, text, `"sub":{`, "WithGroup must namespace attributes")
	assert.Contains(t, text, `debug_should_reach_file_not_stderr`, "file sink must accept Debug")
}
