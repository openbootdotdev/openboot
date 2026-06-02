package shell

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeCatalog returns a resolver that treats only the given names as external.
func fakeCatalog(m map[string]string) func(string) (string, bool) {
	return func(name string) (string, bool) {
		url, ok := m[name]
		return url, ok
	}
}

// withFakes swaps the package-level seams for the duration of a test and
// returns a pointer to a slice that records (url, dest) clone invocations.
func withFakes(t *testing.T, catalog map[string]string) *[][2]string {
	t.Helper()
	var calls [][2]string

	origResolve := resolvePluginURL
	origClone := cloneRunner
	t.Cleanup(func() {
		resolvePluginURL = origResolve
		cloneRunner = origClone
	})

	resolvePluginURL = fakeCatalog(catalog)
	cloneRunner = func(url, dest string) error {
		calls = append(calls, [2]string{url, dest})
		// Simulate a successful clone by creating the destination dir, so a
		// second pass exercises the idempotent skip-if-exists path.
		_ = os.MkdirAll(dest, 0700)
		return nil
	}
	return &calls
}

func customPluginsDir(home string) string {
	return filepath.Join(home, ".oh-my-zsh", "custom", "plugins")
}

func TestCloneExternalPlugins_ClonesKnownExternal(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	calls := withFakes(t, map[string]string{
		"zsh-autosuggestions": "https://github.com/zsh-users/zsh-autosuggestions",
	})

	err := cloneExternalPlugins([]string{"git", "zsh-autosuggestions"}, false)
	require.NoError(t, err)

	require.Len(t, *calls, 1, "only the external plugin should be cloned")
	assert.Equal(t, "https://github.com/zsh-users/zsh-autosuggestions", (*calls)[0][0])
	assert.Equal(t, filepath.Join(customPluginsDir(home), "zsh-autosuggestions"), (*calls)[0][1])
}

func TestCloneExternalPlugins_BuiltinIsNotCloned(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	calls := withFakes(t, map[string]string{
		"zsh-autosuggestions": "https://github.com/zsh-users/zsh-autosuggestions",
	})

	err := cloneExternalPlugins([]string{"git", "docker", "kubectl"}, false)
	require.NoError(t, err)
	assert.Empty(t, *calls, "built-in plugins must never trigger a clone")
}

func TestCloneExternalPlugins_SkipsWhenDestExists(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	calls := withFakes(t, map[string]string{
		"zsh-autosuggestions": "https://github.com/zsh-users/zsh-autosuggestions",
	})
	// Pre-create the destination — already installed.
	require.NoError(t, os.MkdirAll(filepath.Join(customPluginsDir(home), "zsh-autosuggestions"), 0700))

	err := cloneExternalPlugins([]string{"zsh-autosuggestions"}, false)
	require.NoError(t, err)
	assert.Empty(t, *calls, "an already-cloned plugin must be skipped idempotently")
}

func TestCloneExternalPlugins_DryRunDoesNotClone(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	calls := withFakes(t, map[string]string{
		"zsh-autosuggestions": "https://github.com/zsh-users/zsh-autosuggestions",
	})

	err := cloneExternalPlugins([]string{"zsh-autosuggestions"}, true)
	require.NoError(t, err)
	assert.Empty(t, *calls, "dry-run must not clone")
	// And no destination directory should be created.
	_, statErr := os.Stat(filepath.Join(customPluginsDir(home), "zsh-autosuggestions"))
	assert.True(t, os.IsNotExist(statErr), "dry-run must not touch the filesystem")
}

func TestCloneExternalPlugins_NonHTTPSSkipped(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	calls := withFakes(t, map[string]string{
		"evil-plugin": "git@github.com:attacker/evil.git",
	})

	err := cloneExternalPlugins([]string{"evil-plugin"}, false)
	require.NoError(t, err)
	assert.Empty(t, *calls, "non-https repo URLs must be skipped")
}

func TestCloneExternalPlugins_CloneFailureIsNonFatal(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	origResolve := resolvePluginURL
	origClone := cloneRunner
	t.Cleanup(func() {
		resolvePluginURL = origResolve
		cloneRunner = origClone
	})
	resolvePluginURL = fakeCatalog(map[string]string{
		"zsh-autosuggestions": "https://github.com/zsh-users/zsh-autosuggestions",
	})
	cloneRunner = func(url, dest string) error { return errors.New("network down") }

	// A failed clone must not abort the restore.
	err := cloneExternalPlugins([]string{"zsh-autosuggestions"}, false)
	require.NoError(t, err)
}

func TestCloneExternalPluginsFromZshrc_ClonesPluginsFromDotfiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	calls := withFakes(t, map[string]string{
		"zsh-autosuggestions":      "https://github.com/zsh-users/zsh-autosuggestions",
		"fast-syntax-highlighting": "https://github.com/zdharma-continuum/fast-syntax-highlighting",
	})
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".oh-my-zsh"), 0700))
	zshrc := "export ZSH=\"$HOME/.oh-my-zsh\"\nZSH_THEME=\"robbyrussell\"\nplugins=(git helm kubectl zsh-autosuggestions fast-syntax-highlighting)\nsource $ZSH/oh-my-zsh.sh\n"
	require.NoError(t, os.WriteFile(filepath.Join(home, ".zshrc"), []byte(zshrc), 0600))

	require.NoError(t, CloneExternalPluginsFromZshrc(false))

	require.Len(t, *calls, 2, "both external plugins from .zshrc should be cloned; built-ins skipped")
	got := map[string]bool{(*calls)[0][0]: true, (*calls)[1][0]: true}
	assert.True(t, got["https://github.com/zsh-users/zsh-autosuggestions"])
	assert.True(t, got["https://github.com/zdharma-continuum/fast-syntax-highlighting"])
}

func TestCloneExternalPluginsFromZshrc_NoOmzIsNoOp(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	calls := withFakes(t, map[string]string{
		"zsh-autosuggestions": "https://github.com/zsh-users/zsh-autosuggestions",
	})
	// .zshrc present but ~/.oh-my-zsh absent → nothing to clone into.
	require.NoError(t, os.WriteFile(filepath.Join(home, ".zshrc"), []byte("plugins=(zsh-autosuggestions)\n"), 0600))

	require.NoError(t, CloneExternalPluginsFromZshrc(false))
	assert.Empty(t, *calls, "must not clone when oh-my-zsh is not installed")
}

func TestCloneExternalPluginsFromZshrc_MissingZshrcIsNoOp(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	calls := withFakes(t, map[string]string{
		"zsh-autosuggestions": "https://github.com/zsh-users/zsh-autosuggestions",
	})
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".oh-my-zsh"), 0700))

	require.NoError(t, CloneExternalPluginsFromZshrc(false))
	assert.Empty(t, *calls, "a missing .zshrc must be a no-op, not an error")
}

func TestCloneExternalPluginsFromZshrc_UnreadableZshrcWarnsButSucceeds(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	calls := withFakes(t, map[string]string{
		"zsh-autosuggestions": "https://github.com/zsh-users/zsh-autosuggestions",
	})
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".oh-my-zsh"), 0700))
	// A directory at the .zshrc path makes os.ReadFile fail with a non-NotExist
	// error — best-effort plugin setup must warn and continue, not abort.
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".zshrc"), 0700))

	require.NoError(t, CloneExternalPluginsFromZshrc(false), "an unreadable .zshrc must not be fatal")
	assert.Empty(t, *calls)
}

func TestCloneExternalPluginsFromZshrc_DryRunDoesNotClone(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	calls := withFakes(t, map[string]string{
		"zsh-autosuggestions": "https://github.com/zsh-users/zsh-autosuggestions",
	})
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".oh-my-zsh"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(home, ".zshrc"), []byte("plugins=(zsh-autosuggestions)\n"), 0600))

	require.NoError(t, CloneExternalPluginsFromZshrc(true))
	assert.Empty(t, *calls, "dry-run must not clone")
}

func TestCloneExternalPlugins_RestoreWritesBareNames(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	withFakes(t, map[string]string{
		"zsh-autosuggestions": "https://github.com/zsh-users/zsh-autosuggestions",
	})
	// Pre-create ~/.oh-my-zsh so RestoreFromSnapshot's install gate is skipped.
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".oh-my-zsh"), 0700))
	zshrcPath := filepath.Join(home, ".zshrc")
	require.NoError(t, os.WriteFile(zshrcPath, []byte("plugins=(git)\n"), 0600))

	err := RestoreFromSnapshot(true, "agnoster", []string{"git", "zsh-autosuggestions"}, false)
	require.NoError(t, err)

	out, err := os.ReadFile(zshrcPath)
	require.NoError(t, err)
	// The URL never leaks into .zshrc — only bare names.
	assert.Contains(t, string(out), "plugins=(git zsh-autosuggestions)")
	assert.NotContains(t, string(out), "https://")
}
