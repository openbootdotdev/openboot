package shell

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsOhMyZshInstalled_NotInstalled(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	result := IsOhMyZshInstalled()
	assert.False(t, result)
}

func TestIsOhMyZshInstalled_Installed(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	omzDir := filepath.Join(tmpHome, ".oh-my-zsh")
	err := os.MkdirAll(omzDir, 0755)
	require.NoError(t, err)

	result := IsOhMyZshInstalled()
	assert.True(t, result)
}

func TestInstallOhMyZsh_DryRun(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	err := InstallOhMyZsh(true)
	assert.NoError(t, err)

	omzDir := filepath.Join(tmpHome, ".oh-my-zsh")
	_, err = os.Stat(omzDir)
	assert.True(t, os.IsNotExist(err))
}

func TestInstallOhMyZsh_AlreadyInstalled(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	omzDir := filepath.Join(tmpHome, ".oh-my-zsh")
	err := os.MkdirAll(omzDir, 0755)
	require.NoError(t, err)

	err = InstallOhMyZsh(false)
	assert.NoError(t, err)
}

func TestConfigureZshrc_DryRun(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	err := ConfigureZshrc(true)
	assert.NoError(t, err)

	zshrcPath := filepath.Join(tmpHome, ".zshrc")
	_, err = os.Stat(zshrcPath)
	assert.True(t, os.IsNotExist(err))
}

func TestConfigureZshrc_CreatesFile(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	err := ConfigureZshrc(false)
	assert.NoError(t, err)

	zshrcPath := filepath.Join(tmpHome, ".zshrc")
	content, err := os.ReadFile(zshrcPath)
	require.NoError(t, err)

	assert.Contains(t, string(content), "OpenBoot additions")
	assert.Contains(t, string(content), "Homebrew")
	assert.Contains(t, string(content), "alias ls=")
	assert.Contains(t, string(content), "zoxide init")
}

func TestConfigureZshrc_AppendsToExisting(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	zshrcPath := filepath.Join(tmpHome, ".zshrc")
	existingContent := "# Existing config\nexport PATH=/usr/bin:$PATH\n"
	err := os.WriteFile(zshrcPath, []byte(existingContent), 0644)
	require.NoError(t, err)

	err = ConfigureZshrc(false)
	assert.NoError(t, err)

	content, err := os.ReadFile(zshrcPath)
	require.NoError(t, err)

	assert.Contains(t, string(content), "Existing config")
	assert.Contains(t, string(content), "OpenBoot additions")
}

func TestConfigureZshrc_ContainsBrewShellenv(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	err := ConfigureZshrc(false)
	assert.NoError(t, err)

	zshrcPath := filepath.Join(tmpHome, ".zshrc")
	content, err := os.ReadFile(zshrcPath)
	require.NoError(t, err)

	assert.Contains(t, string(content), "/opt/homebrew/bin/brew")
	assert.Contains(t, string(content), "/usr/local/bin/brew")
	assert.Contains(t, string(content), "brew shellenv")
}

func TestConfigureZshrc_ContainsModernAliases(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	err := ConfigureZshrc(false)
	assert.NoError(t, err)

	zshrcPath := filepath.Join(tmpHome, ".zshrc")
	content, err := os.ReadFile(zshrcPath)
	require.NoError(t, err)

	assert.Contains(t, string(content), "alias ls=\"eza --icons\"")
	assert.Contains(t, string(content), "alias cat=\"bat\"")
	assert.Contains(t, string(content), "alias find=\"fd\"")
	assert.Contains(t, string(content), "alias grep=\"rg\"")
	assert.Contains(t, string(content), "alias top=\"btop\"")
}

func TestConfigureZshrc_ContainsGitAliases(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	err := ConfigureZshrc(false)
	assert.NoError(t, err)

	zshrcPath := filepath.Join(tmpHome, ".zshrc")
	content, err := os.ReadFile(zshrcPath)
	require.NoError(t, err)

	assert.Contains(t, string(content), "alias gs=\"git status\"")
	assert.Contains(t, string(content), "alias gd=\"git diff\"")
	assert.Contains(t, string(content), "alias gl=\"lazygit\"")
}

func TestConfigureZshrc_ContainsFzf(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	err := ConfigureZshrc(false)
	assert.NoError(t, err)

	zshrcPath := filepath.Join(tmpHome, ".zshrc")
	content, err := os.ReadFile(zshrcPath)
	require.NoError(t, err)

	assert.Contains(t, string(content), "~/.fzf.zsh")
}

func TestSetDefaultShell_DryRun(t *testing.T) {
	err := SetDefaultShell(true)
	assert.NoError(t, err)
}
