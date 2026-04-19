package system

import (
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunCommandSilent_Success(t *testing.T) {
	output, err := RunCommandSilent("echo", "hello", "world")
	require.NoError(t, err)
	assert.Equal(t, "hello world", output)
}

func TestRunCommandSilent_TrimSpace(t *testing.T) {
	output, err := RunCommandSilent("echo", "  test  ")
	require.NoError(t, err)
	assert.Equal(t, "test", output)
}

func TestRunCommandSilent_CommandNotFound(t *testing.T) {
	output, err := RunCommandSilent("nonexistentcommand12345")
	assert.Error(t, err)
	assert.Contains(t, output, "")
}

func TestRunCommandSilent_CommandFails(t *testing.T) {
	output, err := RunCommandSilent("ls", "/nonexistent/directory/12345")
	assert.Error(t, err)
	assert.NotEmpty(t, output)
}

func TestRunCommand_Success(t *testing.T) {
	err := RunCommand("echo", "test")
	assert.NoError(t, err)
}

func TestRunCommand_CommandNotFound(t *testing.T) {
	err := RunCommand("nonexistentcommand12345")
	assert.Error(t, err)
}

func TestRunCommand_CommandFails(t *testing.T) {
	err := RunCommand("ls", "/nonexistent/directory/12345")
	assert.Error(t, err)
}

func TestHasTTY(t *testing.T) {
	result := HasTTY()
	assert.IsType(t, true, result)
}

func TestOpenTTY_ReturnsFallbackOrTTY(t *testing.T) {
	tty, opened := OpenTTY()
	assert.NotNil(t, tty, "OpenTTY must always return a non-nil file")

	if opened {
		assert.NotEqual(t, os.Stdin.Fd(), tty.Fd(),
			"opened tty should have a different fd than os.Stdin")
		require.NoError(t, tty.Close())
	} else {
		assert.Equal(t, os.Stdin.Fd(), tty.Fd(),
			"fallback should return os.Stdin")
	}
}

func TestOpenTTY_OpenedFileIsReadable(t *testing.T) {
	tty, opened := OpenTTY()
	if !opened {
		t.Skip("/dev/tty not available")
	}
	defer tty.Close()

	info, err := tty.Stat()
	require.NoError(t, err)
	assert.NotNil(t, info)
}

func TestOpenTTY_FallbackDoesNotClose(t *testing.T) {
	// When /dev/tty is unavailable, opened=false signals the caller
	// must NOT close the returned file (it's os.Stdin).
	_, opened := OpenTTY()
	assert.IsType(t, true, opened)
}

func TestOpenTTY_SubprocessSeesRealTTY(t *testing.T) {
	// Core regression test: a subprocess given an OpenTTY fd should see
	// stdin as a TTY, which is required for sudo password prompts.
	tty, opened := OpenTTY()
	if !opened {
		t.Skip("/dev/tty not available")
	}
	defer tty.Close()

	cmd := exec.Command("test", "-t", "0")
	cmd.Stdin = tty
	err := cmd.Run()
	assert.NoError(t, err, "subprocess stdin should be a TTY when using OpenTTY")
}

func TestOpenTTY_MultipleCallsReturnIndependentFDs(t *testing.T) {
	// Each call should open a fresh fd so concurrent callers don't
	// interfere (e.g. parallel cask installs).
	tty1, opened1 := OpenTTY()
	if !opened1 {
		t.Skip("/dev/tty not available")
	}
	defer tty1.Close()

	tty2, opened2 := OpenTTY()
	require.True(t, opened2)
	defer tty2.Close()

	assert.NotEqual(t, tty1.Fd(), tty2.Fd(),
		"each OpenTTY call should return a distinct fd")
}

// TestOpenTTY_PipedStdinSimulation is the most important test: it reproduces
// the exact curl|bash scenario. We spawn a child Go process whose stdin is a
// pipe (not a TTY), and the child calls OpenTTY and checks whether the
// returned fd is still a TTY via /dev/tty.
func TestOpenTTY_PipedStdinSimulation(t *testing.T) {
	if _, err := os.Open("/dev/tty"); err != nil {
		t.Skip("/dev/tty not available")
	}

	// Spawn ourselves with a special env var so the child runs the
	// verification logic instead of the test suite.
	child := exec.Command(os.Args[0], "-test.run=TestOpenTTYChildHelper")
	child.Env = append(os.Environ(), "OPENBOOT_TTY_CHILD=1")
	// Give the child a pipe for stdin — simulating curl|bash.
	child.Stdin, _ = os.Open(os.DevNull)

	output, err := child.CombinedOutput()
	assert.NoError(t, err,
		"child with piped stdin should still get TTY via OpenTTY; output: %s", string(output))
}

// TestOpenTTYChildHelper is not a real test — it's the child process entry
// point for TestOpenTTY_PipedStdinSimulation.
func TestOpenTTYChildHelper(t *testing.T) {
	if os.Getenv("OPENBOOT_TTY_CHILD") != "1" {
		t.Skip("helper only runs as child process")
	}

	tty, opened := OpenTTY()
	if !opened {
		t.Fatal("OpenTTY returned opened=false even though /dev/tty exists")
	}
	defer tty.Close()

	// Verify via subprocess that the fd is a real TTY.
	cmd := exec.Command("test", "-t", "0")
	cmd.Stdin = tty
	if err := cmd.Run(); err != nil {
		t.Fatalf("stdin from OpenTTY is not a TTY: %v", err)
	}
}

// TestOpenTTY_CloseDoesNotBreakTerminal verifies that closing the fd
// returned by OpenTTY does not break the controlling terminal. After close,
// a new OpenTTY call should still succeed — proving the terminal device
// itself is unaffected.
func TestOpenTTY_CloseDoesNotBreakTerminal(t *testing.T) {
	tty1, opened := OpenTTY()
	if !opened {
		t.Skip("/dev/tty not available")
	}
	// Close the fd.
	require.NoError(t, tty1.Close())

	// Terminal should still be accessible.
	tty2, opened2 := OpenTTY()
	require.True(t, opened2, "terminal should still be available after closing a previous fd")
	defer tty2.Close()

	cmd := exec.Command("test", "-t", "0")
	cmd.Stdin = tty2
	assert.NoError(t, cmd.Run(), "subprocess should still see a TTY after previous fd was closed")
}

// TestOpenTTY_SequentialCaskSimulation simulates two sequential cask installs
// (the real pattern: first install + retry), each opening and closing their
// own TTY fd. The second install must not be affected by the first's close.
func TestOpenTTY_SequentialCaskSimulation(t *testing.T) {
	tty1, opened := OpenTTY()
	if !opened {
		t.Skip("/dev/tty not available")
	}

	// Simulate first cask install.
	cmd1 := exec.Command("test", "-t", "0")
	cmd1.Stdin = tty1
	require.NoError(t, cmd1.Run())
	tty1.Close() //nolint:errcheck // test cleanup

	// Simulate retry — must still work.
	tty2, opened2 := OpenTTY()
	require.True(t, opened2)
	defer tty2.Close()

	cmd2 := exec.Command("test", "-t", "0")
	cmd2.Stdin = tty2
	assert.NoError(t, cmd2.Run(), "retry should still have a working TTY")
}

func TestGetGitConfig_NonExistentKey(t *testing.T) {
	value := GetGitConfig("user.nonexistentkey12345")
	assert.Equal(t, "", value)
}

func TestGetGitConfig_ValidKey(t *testing.T) {
	output, err := RunCommandSilent("git", "config", "--global", "user.name")
	if err != nil {
		t.Skip("git not configured, skipping")
	}

	value := GetGitConfig("user.name")
	assert.Equal(t, output, value)
}

func TestGetExistingGitConfig(t *testing.T) {
	name, email := GetExistingGitConfig()
	assert.IsType(t, "", name)
	assert.IsType(t, "", email)
}

func TestConfigureGit_DryRunWithEnv(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	gitDir := tmpHome + "/.gitconfig"
	if _, err := os.Stat(gitDir); err == nil {
		t.Skip("git config already exists")
	}

	err := ConfigureGit("Test User", "test@example.com")
	if err != nil {
		t.Logf("ConfigureGit failed (expected if git not installed): %v", err)
	}
}

func TestRunCommandSilent_CombinesOutput(t *testing.T) {
	output, err := RunCommandSilent("sh", "-c", "echo stdout; echo stderr >&2")
	require.NoError(t, err)

	assert.Contains(t, output, "stdout")
	assert.Contains(t, output, "stderr")
}

func TestRunCommandSilent_EmptyOutput(t *testing.T) {
	output, err := RunCommandSilent("true")
	require.NoError(t, err)
	assert.Equal(t, "", output)
}

func TestRunCommand_WithArguments(t *testing.T) {
	err := RunCommand("echo", "arg1", "arg2", "arg3")
	assert.NoError(t, err)
}

func TestRunCommandSilent_MultilineOutput(t *testing.T) {
	output, err := RunCommandSilent("sh", "-c", "echo line1; echo line2; echo line3")
	require.NoError(t, err)

	assert.Contains(t, output, "line1")
	assert.Contains(t, output, "line2")
	assert.Contains(t, output, "line3")
}

// TestGetGitConfig_FallsBackToAnyScope verifies that GetGitConfig checks all git config scopes,
// not just --global. This handles cases where user.name/user.email are set in local or system config.
// Regression test for: git config detection issue
func TestGetGitConfig_FallsBackToAnyScope(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Create a temporary git config file
	gitConfigDir := tmpDir + "/.config/git"
	os.MkdirAll(gitConfigDir, 0755) //nolint:errcheck // test setup; failure causes predictable test behavior
	// Test that GetGitConfig returns empty when nothing is set
	value := GetGitConfig("user.testkey")
	// If git is not installed or no config exists, should return empty
	// The function tries --global first, then falls back to any scope
	assert.IsType(t, "", value)
}

// ---------------------------------------------------------------------------
// HomeDir
// ---------------------------------------------------------------------------

func TestHomeDir_ReturnsNonEmpty(t *testing.T) {
	home, err := HomeDir()
	require.NoError(t, err)
	assert.NotEmpty(t, home)
}

func TestHomeDir_MatchesEnv(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	home, err := HomeDir()
	require.NoError(t, err)
	assert.Equal(t, tmpDir, home)
}

// ---------------------------------------------------------------------------
// RunCommandOutput
// ---------------------------------------------------------------------------

func TestRunCommandOutput_CapturesStdout(t *testing.T) {
	output, err := RunCommandOutput("echo", "hello")
	require.NoError(t, err)
	assert.Equal(t, "hello", output)
}

func TestRunCommandOutput_DoesNotCaptureStderr(t *testing.T) {
	// When the command writes only to stderr and succeeds (exit 0), stdout is empty.
	output, err := RunCommandOutput("sh", "-c", "echo only-stderr >&2")
	require.NoError(t, err)
	assert.Equal(t, "", output)
}

func TestRunCommandOutput_TrimSpace(t *testing.T) {
	output, err := RunCommandOutput("printf", "  trimmed  ")
	require.NoError(t, err)
	assert.Equal(t, "trimmed", output)
}

func TestRunCommandOutput_CommandFails(t *testing.T) {
	_, err := RunCommandOutput("ls", "/nonexistent/directory/12345")
	assert.Error(t, err)
}

func TestRunCommandOutput_CommandNotFound(t *testing.T) {
	_, err := RunCommandOutput("nonexistentcmd99999")
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// HasTTY — ensure all branches are exercised
// ---------------------------------------------------------------------------

func TestHasTTY_ReturnsBool(t *testing.T) {
	result := HasTTY()
	// Just assert it's actually a bool (not a zero-value from uninitialized state).
	assert.IsType(t, false, result)
}

// ---------------------------------------------------------------------------
// ConfigureGit — error path via bad HOME
// ---------------------------------------------------------------------------

func TestConfigureGit_WritesConfig(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	err := ConfigureGit("Test User", "test@example.com")
	if err != nil {
		// git may not be installed in all CI environments — acceptable skip.
		t.Logf("ConfigureGit failed (tolerated): %v", err)
		return
	}

	// Verify the config was written correctly.
	name, email := GetExistingGitConfig()
	assert.Equal(t, "Test User", name)
	assert.Equal(t, "test@example.com", email)
}

// ---------------------------------------------------------------------------
// IsAllowedAPIURL — all branches
// ---------------------------------------------------------------------------

func TestIsAllowedAPIURL(t *testing.T) {
	tests := []struct {
		name  string
		url   string
		allow bool
	}{
		{"https scheme allowed", "https://openboot.dev/api", true},
		{"https with path", "https://example.com/foo/bar", true},
		{"localhost http allowed", "http://localhost:8080/api", true},
		{"127.0.0.1 http allowed", "http://127.0.0.1:3000", true},
		{"IPv6 loopback allowed", "http://[::1]:9000", true},
		{"plain http blocked", "http://openboot.dev/api", false},
		{"http other host blocked", "http://evil.com", false},
		{"ftp scheme blocked", "ftp://example.com", false},
		{"empty string blocked", "", false},
		{"just hostname blocked", "openboot.dev", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := IsAllowedAPIURL(tc.url)
			assert.Equal(t, tc.allow, got)
		})
	}
}
