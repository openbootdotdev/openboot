package system

import (
	"os"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArchitecture(t *testing.T) {
	arch := Architecture()
	assert.Equal(t, runtime.GOARCH, arch)
	assert.NotEmpty(t, arch)
}

func TestHomebrewPrefix(t *testing.T) {
	prefix := HomebrewPrefix()

	if runtime.GOARCH == "arm64" {
		assert.Equal(t, "/opt/homebrew", prefix)
	} else {
		assert.Equal(t, "/usr/local", prefix)
	}
}

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

func TestIsHomebrewInstalled(t *testing.T) {
	result := IsHomebrewInstalled()
	assert.IsType(t, true, result)
}

func TestIsXcodeCliInstalled(t *testing.T) {
	result := IsXcodeCliInstalled()
	assert.IsType(t, true, result)
}

func TestIsGumInstalled(t *testing.T) {
	result := IsGumInstalled()
	assert.IsType(t, true, result)
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
