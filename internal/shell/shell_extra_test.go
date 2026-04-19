package shell

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// validateShellIdentifier
// ---------------------------------------------------------------------------

func TestValidateShellIdentifier_Valid(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{"empty is ok", ""},
		{"simple word", "robbyrussell"},
		{"with digits", "plugin123"},
		{"with hyphen", "zsh-autosuggestions"},
		{"with underscore", "my_plugin"},
		{"with dot", "plugin.v2"},
		{"mixed", "My-Plugin_2.0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateShellIdentifier(tt.value, "test")
			assert.NoError(t, err)
		})
	}
}

func TestValidateShellIdentifier_Invalid(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{"space", "bad value"},
		{"semicolon", "bad;value"},
		{"dollar sign", "$BAD"},
		{"backtick", "`cmd`"},
		{"slash", "a/b"},
		{"paren", "a(b)"},
		{"newline embedded", "foo\nbar"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateShellIdentifier(tt.value, "label")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "invalid label")
		})
	}
}

// ---------------------------------------------------------------------------
// buildRestoreBlock
// ---------------------------------------------------------------------------

func TestBuildRestoreBlock_InvalidTheme(t *testing.T) {
	_, err := buildRestoreBlock("bad theme!", []string{"git"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ZSH_THEME")
}

func TestBuildRestoreBlock_InvalidPlugin(t *testing.T) {
	_, err := buildRestoreBlock("agnoster", []string{"good-plugin", "bad plugin!"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plugin")
}

func TestBuildRestoreBlock_NoThemeNoPlugins(t *testing.T) {
	block, err := buildRestoreBlock("", nil)
	require.NoError(t, err)
	assert.Contains(t, block, restoreBlockStart)
	assert.Contains(t, block, restoreBlockEnd)
	assert.NotContains(t, block, "ZSH_THEME")
	assert.NotContains(t, block, "plugins=")
}

func TestBuildRestoreBlock_Full(t *testing.T) {
	block, err := buildRestoreBlock("agnoster", []string{"git", "docker"})
	require.NoError(t, err)
	assert.Contains(t, block, `ZSH_THEME="agnoster"`)
	assert.Contains(t, block, `plugins=(git docker)`)
	assert.True(t, strings.HasPrefix(block, restoreBlockStart))
}

// ---------------------------------------------------------------------------
// patchZshrcBlock
// ---------------------------------------------------------------------------

func TestPatchZshrcBlock_FreshFile_NoExistingBlock(t *testing.T) {
	home := t.TempDir()
	zshrcPath := filepath.Join(home, ".zshrc")

	initial := `export ZSH="$HOME/.oh-my-zsh"
ZSH_THEME="robbyrussell"
plugins=(git)
source $ZSH/oh-my-zsh.sh
`
	require.NoError(t, os.WriteFile(zshrcPath, []byte(initial), 0600))

	require.NoError(t, patchZshrcBlock(zshrcPath, "agnoster", []string{"git", "docker"}))

	content, err := os.ReadFile(zshrcPath)
	require.NoError(t, err)
	s := string(content)

	assert.Contains(t, s, restoreBlockStart)
	assert.Contains(t, s, restoreBlockEnd)
	assert.Contains(t, s, `ZSH_THEME="agnoster"`)
	assert.Contains(t, s, `plugins=(git docker)`)
	// Loose declarations should be stripped when block is written fresh.
	assert.NotContains(t, s, `ZSH_THEME="robbyrussell"`)
}

func TestPatchZshrcBlock_ReplacesExistingBlock(t *testing.T) {
	home := t.TempDir()
	zshrcPath := filepath.Join(home, ".zshrc")

	initial := `export ZSH="$HOME/.oh-my-zsh"
# >>> OpenBoot-Restore
ZSH_THEME="old-theme"
plugins=(git)
# <<< OpenBoot-Restore
source $ZSH/oh-my-zsh.sh
`
	require.NoError(t, os.WriteFile(zshrcPath, []byte(initial), 0600))

	require.NoError(t, patchZshrcBlock(zshrcPath, "new-theme", []string{"git", "zsh-autosuggestions"}))

	content, err := os.ReadFile(zshrcPath)
	require.NoError(t, err)
	s := string(content)

	assert.Equal(t, 1, strings.Count(s, restoreBlockStart), "block should appear exactly once")
	assert.Contains(t, s, `ZSH_THEME="new-theme"`)
	assert.Contains(t, s, `plugins=(git zsh-autosuggestions)`)
	assert.NotContains(t, s, `ZSH_THEME="old-theme"`)
}

func TestPatchZshrcBlock_NoTrailingNewlineHandled(t *testing.T) {
	home := t.TempDir()
	zshrcPath := filepath.Join(home, ".zshrc")

	// File without trailing newline.
	require.NoError(t, os.WriteFile(zshrcPath, []byte(`source $ZSH/oh-my-zsh.sh`), 0600))

	require.NoError(t, patchZshrcBlock(zshrcPath, "agnoster", []string{"git"}))

	content, err := os.ReadFile(zshrcPath)
	require.NoError(t, err)
	s := string(content)
	assert.Contains(t, s, restoreBlockStart)
	// Block sentinel must be on its own line.
	for _, line := range strings.Split(s, "\n") {
		if strings.Contains(line, "oh-my-zsh.sh") {
			assert.NotContains(t, line, restoreBlockStart)
		}
	}
}

func TestPatchZshrcBlock_InvalidIdentifierReturnsError(t *testing.T) {
	home := t.TempDir()
	zshrcPath := filepath.Join(home, ".zshrc")
	require.NoError(t, os.WriteFile(zshrcPath, []byte("# placeholder\n"), 0600))

	err := patchZshrcBlock(zshrcPath, "theme with spaces!", []string{"git"})
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// EnsureBrewShellenv
// ---------------------------------------------------------------------------

func TestEnsureBrewShellenv_NoBrew_ReturnsNil(t *testing.T) {
	// On machines without /opt/homebrew/bin/brew the function should be a no-op.
	if _, err := os.Stat("/opt/homebrew/bin/brew"); err == nil {
		t.Skip("real Homebrew present; skipping no-brew path")
	}

	home := t.TempDir()
	t.Setenv("HOME", home)

	err := EnsureBrewShellenv(false)
	assert.NoError(t, err)

	// .zshrc should not have been created.
	_, statErr := os.Stat(filepath.Join(home, ".zshrc"))
	assert.True(t, os.IsNotExist(statErr))
}

func TestEnsureBrewShellenv_WithBrew_CreatesZshrc(t *testing.T) {
	if _, err := os.Stat("/opt/homebrew/bin/brew"); os.IsNotExist(err) {
		t.Skip("no /opt/homebrew/bin/brew on this machine")
	}

	home := t.TempDir()
	t.Setenv("HOME", home)

	err := EnsureBrewShellenv(false)
	require.NoError(t, err)

	zshrcPath := filepath.Join(home, ".zshrc")
	content, err := os.ReadFile(zshrcPath)
	require.NoError(t, err)
	assert.Contains(t, string(content), "brew shellenv")
}

func TestEnsureBrewShellenv_WithBrew_Idempotent(t *testing.T) {
	if _, err := os.Stat("/opt/homebrew/bin/brew"); os.IsNotExist(err) {
		t.Skip("no /opt/homebrew/bin/brew on this machine")
	}

	home := t.TempDir()
	t.Setenv("HOME", home)

	// Call twice — shellenv line must appear exactly once.
	require.NoError(t, EnsureBrewShellenv(false))
	require.NoError(t, EnsureBrewShellenv(false))

	content, err := os.ReadFile(filepath.Join(home, ".zshrc"))
	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(string(content), "brew shellenv"),
		"shellenv line must appear exactly once after two calls")
}

func TestEnsureBrewShellenv_WithBrew_AlreadyPresent_NoDuplicate(t *testing.T) {
	if _, err := os.Stat("/opt/homebrew/bin/brew"); os.IsNotExist(err) {
		t.Skip("no /opt/homebrew/bin/brew on this machine")
	}

	home := t.TempDir()
	t.Setenv("HOME", home)

	// Pre-populate .zshrc with the shellenv line.
	zshrcPath := filepath.Join(home, ".zshrc")
	require.NoError(t, os.WriteFile(zshrcPath, []byte(brewShellenvLine+"\n"), 0600))

	require.NoError(t, EnsureBrewShellenv(false))

	content, err := os.ReadFile(zshrcPath)
	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(string(content), "brew shellenv"))
}

func TestEnsureBrewShellenv_WithBrew_DryRun_NoWrite(t *testing.T) {
	if _, err := os.Stat("/opt/homebrew/bin/brew"); os.IsNotExist(err) {
		t.Skip("no /opt/homebrew/bin/brew on this machine")
	}

	home := t.TempDir()
	t.Setenv("HOME", home)

	err := EnsureBrewShellenv(true)
	require.NoError(t, err)

	// In dry-run mode with no existing .zshrc, no file should be written.
	_, statErr := os.Stat(filepath.Join(home, ".zshrc"))
	assert.True(t, os.IsNotExist(statErr), "dry-run must not create .zshrc")
}

func TestEnsureBrewShellenv_WithBrew_ExistingZshrc_DryRun(t *testing.T) {
	if _, err := os.Stat("/opt/homebrew/bin/brew"); os.IsNotExist(err) {
		t.Skip("no /opt/homebrew/bin/brew on this machine")
	}

	home := t.TempDir()
	t.Setenv("HOME", home)

	zshrcPath := filepath.Join(home, ".zshrc")
	original := "# my zshrc\n"
	require.NoError(t, os.WriteFile(zshrcPath, []byte(original), 0600))

	require.NoError(t, EnsureBrewShellenv(true))

	// Content must be unchanged after dry-run.
	content, err := os.ReadFile(zshrcPath)
	require.NoError(t, err)
	assert.Equal(t, original, string(content))
}

func TestEnsureBrewShellenv_WithBrew_AppendToExistingZshrc(t *testing.T) {
	if _, err := os.Stat("/opt/homebrew/bin/brew"); os.IsNotExist(err) {
		t.Skip("no /opt/homebrew/bin/brew on this machine")
	}

	home := t.TempDir()
	t.Setenv("HOME", home)

	zshrcPath := filepath.Join(home, ".zshrc")
	require.NoError(t, os.WriteFile(zshrcPath, []byte("# existing line\n"), 0600))

	require.NoError(t, EnsureBrewShellenv(false))

	content, err := os.ReadFile(zshrcPath)
	require.NoError(t, err)
	s := string(content)
	assert.Contains(t, s, "brew shellenv")
	assert.Contains(t, s, "# existing line")
}

// ---------------------------------------------------------------------------
// RestoreFromSnapshot — additional paths
// ---------------------------------------------------------------------------

func TestRestoreFromSnapshot_DryRun_NoZshrc_NoWrite(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// OMZ dir exists so IsOhMyZshInstalled() returns true.
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".oh-my-zsh"), 0755))

	err := RestoreFromSnapshot(true, "agnoster", []string{"git"}, true)
	require.NoError(t, err)

	_, statErr := os.Stat(filepath.Join(home, ".zshrc"))
	assert.True(t, os.IsNotExist(statErr), "dry-run must not create .zshrc")
}

func TestRestoreFromSnapshot_DryRun_WithZshrc_NoWrite(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	require.NoError(t, os.MkdirAll(filepath.Join(home, ".oh-my-zsh"), 0755))

	zshrcPath := filepath.Join(home, ".zshrc")
	original := "export ZSH=\"$HOME/.oh-my-zsh\"\nZSH_THEME=\"robbyrussell\"\nplugins=(git)\n"
	require.NoError(t, os.WriteFile(zshrcPath, []byte(original), 0600))

	err := RestoreFromSnapshot(true, "agnoster", []string{"git", "docker"}, true)
	require.NoError(t, err)

	content, err := os.ReadFile(zshrcPath)
	require.NoError(t, err)
	// File must be unchanged under dry-run.
	assert.Equal(t, original, string(content))
}

func TestRestoreFromSnapshot_ValidatesThemeOnCreate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	require.NoError(t, os.MkdirAll(filepath.Join(home, ".oh-my-zsh"), 0755))
	// No .zshrc — creation path, which validates identifiers.

	err := RestoreFromSnapshot(true, "bad theme!", nil, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ZSH_THEME")
}

func TestRestoreFromSnapshot_ValidatesPluginsOnCreate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	require.NoError(t, os.MkdirAll(filepath.Join(home, ".oh-my-zsh"), 0755))

	err := RestoreFromSnapshot(true, "agnoster", []string{"good", "bad plugin!"}, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plugin")
}
