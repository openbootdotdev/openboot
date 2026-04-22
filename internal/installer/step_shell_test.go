package installer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupDotfilesWithZshrc creates ~/.dotfiles/.zshrc (flat layout, no .git)
// so dotfiles.Clone treats the dir as already present and skips cloning,
// and dotfiles.Link takes the linkDirect path (no stow required).
func setupDotfilesWithZshrc(t *testing.T, zshrcContent string) string {
	t.Helper()
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	dotfilesDir := filepath.Join(tmpHome, ".dotfiles")
	require.NoError(t, os.MkdirAll(dotfilesDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dotfilesDir, ".zshrc"), []byte(zshrcContent), 0o644))
	return tmpHome
}

func TestApplyDotfiles_InstallsOMZWhenDotfilesRequireIt(t *testing.T) {
	setupDotfilesWithZshrc(t, "source $ZSH/oh-my-zsh.sh\n")

	called := 0
	orig := installOhMyZshFunc
	installOhMyZshFunc = func(dryRun bool) error {
		called++
		return nil
	}
	t.Cleanup(func() { installOhMyZshFunc = orig })

	plan := InstallPlan{
		DotfilesURL:    "https://github.com/user/dotfiles",
		InstallOhMyZsh: false,
	}
	require.NoError(t, applyDotfiles(plan, NopReporter{}))
	assert.Equal(t, 1, called, "OMZ installer should be invoked when dotfiles reference it")
}

func TestApplyDotfiles_SkipsOMZWhenDotfilesDontReferenceIt(t *testing.T) {
	setupDotfilesWithZshrc(t, "# no oh-my-zsh here\nalias ll='ls -la'\n")

	called := 0
	orig := installOhMyZshFunc
	installOhMyZshFunc = func(dryRun bool) error {
		called++
		return nil
	}
	t.Cleanup(func() { installOhMyZshFunc = orig })

	plan := InstallPlan{
		DotfilesURL:    "https://github.com/user/dotfiles",
		InstallOhMyZsh: false,
	}
	require.NoError(t, applyDotfiles(plan, NopReporter{}))
	assert.Equal(t, 0, called, "OMZ installer should not run when dotfiles don't reference OMZ")
}

func TestApplyDotfiles_SkipsOMZWhenShellStepAlreadyTried(t *testing.T) {
	// Even when dotfiles reference OMZ, if the shell step was supposed to
	// install it (and failed), don't retry — we'd just fail again.
	setupDotfilesWithZshrc(t, "source $ZSH/oh-my-zsh.sh\n")

	called := 0
	orig := installOhMyZshFunc
	installOhMyZshFunc = func(dryRun bool) error {
		called++
		return nil
	}
	t.Cleanup(func() { installOhMyZshFunc = orig })

	plan := InstallPlan{
		DotfilesURL:    "https://github.com/user/dotfiles",
		InstallOhMyZsh: true, // shell step already handled OMZ
	}
	require.NoError(t, applyDotfiles(plan, NopReporter{}))
	assert.Equal(t, 0, called, "should not retry OMZ install when shell step already attempted")
}

func TestApplyDotfiles_SkipsOMZInDryRun(t *testing.T) {
	setupDotfilesWithZshrc(t, "source $ZSH/oh-my-zsh.sh\n")

	called := 0
	orig := installOhMyZshFunc
	installOhMyZshFunc = func(dryRun bool) error {
		called++
		return nil
	}
	t.Cleanup(func() { installOhMyZshFunc = orig })

	plan := InstallPlan{
		DotfilesURL:    "https://github.com/user/dotfiles",
		InstallOhMyZsh: false,
		DryRun:         true,
	}
	require.NoError(t, applyDotfiles(plan, NopReporter{}))
	assert.Equal(t, 0, called, "dry-run must not invoke OMZ installer")
}
