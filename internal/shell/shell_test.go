package shell

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

func TestSetDefaultShell_DryRun(t *testing.T) {
	err := SetDefaultShell(true)
	assert.NoError(t, err)
}

func TestRestoreFromSnapshot_NoOhMyZsh(t *testing.T) {
	err := RestoreFromSnapshot(false, "robbyrussell", []string{"git"}, true)
	assert.NoError(t, err)
}

func TestRestoreFromSnapshot_DryRun_ExistingZshrc(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	zshrcPath := filepath.Join(home, ".zshrc")
	content := `export ZSH="$HOME/.oh-my-zsh"
ZSH_THEME="robbyrussell"
plugins=(git)
source $ZSH/oh-my-zsh.sh
`
	require.NoError(t, os.WriteFile(zshrcPath, []byte(content), 0644))

	omzDir := filepath.Join(home, ".oh-my-zsh")
	require.NoError(t, os.MkdirAll(omzDir, 0755))

	err := RestoreFromSnapshot(true, "agnoster", []string{"git", "zsh-autosuggestions"}, true)
	assert.NoError(t, err)

	result, err := os.ReadFile(zshrcPath)
	require.NoError(t, err)
	assert.Contains(t, string(result), `ZSH_THEME="robbyrussell"`)
	assert.Contains(t, string(result), `plugins=(git)`)
}

func TestRestoreFromSnapshot_UpdatesExistingZshrc(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	zshrcPath := filepath.Join(home, ".zshrc")
	content := `export ZSH="$HOME/.oh-my-zsh"
ZSH_THEME="robbyrussell"
plugins=(git)
source $ZSH/oh-my-zsh.sh
`
	require.NoError(t, os.WriteFile(zshrcPath, []byte(content), 0644))

	omzDir := filepath.Join(home, ".oh-my-zsh")
	require.NoError(t, os.MkdirAll(omzDir, 0755))

	err := RestoreFromSnapshot(true, "agnoster", []string{"git", "zsh-autosuggestions", "docker"}, false)
	assert.NoError(t, err)

	result, err := os.ReadFile(zshrcPath)
	require.NoError(t, err)
	assert.Contains(t, string(result), `ZSH_THEME="agnoster"`)
	assert.Contains(t, string(result), `plugins=(git zsh-autosuggestions docker)`)
	assert.NotContains(t, string(result), `ZSH_THEME="robbyrussell"`)
}

func TestRestoreFromSnapshot_CreatesZshrcIfMissing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	omzDir := filepath.Join(home, ".oh-my-zsh")
	require.NoError(t, os.MkdirAll(omzDir, 0755))

	zshrcPath := filepath.Join(home, ".zshrc")
	assert.NoFileExists(t, zshrcPath)

	err := RestoreFromSnapshot(true, "powerlevel10k", []string{"git", "docker"}, false)
	assert.NoError(t, err)

	result, err := os.ReadFile(zshrcPath)
	require.NoError(t, err)
	assert.Contains(t, string(result), `ZSH_THEME="powerlevel10k"`)
	assert.Contains(t, string(result), `plugins=(git docker)`)
	assert.Contains(t, string(result), `source $ZSH/oh-my-zsh.sh`)
}

func TestRestoreFromSnapshot_EmptyThemeAndPlugins(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	zshrcPath := filepath.Join(home, ".zshrc")
	content := `export ZSH="$HOME/.oh-my-zsh"
ZSH_THEME="robbyrussell"
plugins=(git)
source $ZSH/oh-my-zsh.sh
`
	require.NoError(t, os.WriteFile(zshrcPath, []byte(content), 0644))

	omzDir := filepath.Join(home, ".oh-my-zsh")
	require.NoError(t, os.MkdirAll(omzDir, 0755))

	err := RestoreFromSnapshot(true, "", nil, false)
	assert.NoError(t, err)

	result, err := os.ReadFile(zshrcPath)
	require.NoError(t, err)
	assert.Contains(t, string(result), `ZSH_THEME="robbyrussell"`)
	assert.Contains(t, string(result), `plugins=(git)`)
}

func TestRestoreFromSnapshot_NoTrailingNewline(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	zshrcPath := filepath.Join(home, ".zshrc")
	require.NoError(t, os.WriteFile(zshrcPath, []byte(`source $ZSH/oh-my-zsh.sh`), 0644))
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".oh-my-zsh"), 0755))

	require.NoError(t, RestoreFromSnapshot(true, "agnoster", []string{"git"}, false))

	content, err := os.ReadFile(zshrcPath)
	require.NoError(t, err)

	assert.Contains(t, string(content), restoreBlockStart)
	lines := strings.Split(string(content), "\n")
	for i, line := range lines {
		if strings.HasSuffix(line, "oh-my-zsh.sh") {
			assert.NotContains(t, line, restoreBlockStart,
				"block sentinel must not be joined to previous line (line %d)", i)
			break
		}
	}
}

func TestRestoreFromSnapshot_Idempotent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	zshrcPath := filepath.Join(home, ".zshrc")
	initial := `export ZSH="$HOME/.oh-my-zsh"
ZSH_THEME="robbyrussell"
plugins=(git)
source $ZSH/oh-my-zsh.sh
`
	require.NoError(t, os.WriteFile(zshrcPath, []byte(initial), 0644))
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".oh-my-zsh"), 0755))

	require.NoError(t, RestoreFromSnapshot(true, "agnoster", []string{"git", "docker"}, false))
	require.NoError(t, RestoreFromSnapshot(true, "agnoster", []string{"git", "docker"}, false))
	require.NoError(t, RestoreFromSnapshot(true, "agnoster", []string{"git", "docker"}, false))

	content, err := os.ReadFile(zshrcPath)
	require.NoError(t, err)

	assert.Equal(t, 1, strings.Count(string(content), restoreBlockStart),
		"restore block should appear exactly once after repeated calls")
	assert.Contains(t, string(content), `ZSH_THEME="agnoster"`)
	assert.NotContains(t, string(content), `ZSH_THEME="robbyrussell"`)
}

// --- Hash verification tests for InstallOhMyZsh ---

// hashOf returns the lowercase hex SHA256 of data.
func hashOf(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// serveScript starts an httptest server that returns body as the script payload.
// It restores omzInstallURL when the test completes.
func serveScript(t *testing.T, body []byte, statusCode int) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(statusCode)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	orig := omzInstallURL
	omzInstallURL = srv.URL
	t.Cleanup(func() { omzInstallURL = orig })
}

func TestInstallOhMyZsh_HashMismatch(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Serve a script whose hash does NOT match knownOMZInstallHash.
	fakeScript := []byte("#!/bin/sh\necho fake\n")
	serveScript(t, fakeScript, http.StatusOK)

	err := InstallOhMyZsh(false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "hash mismatch")
	assert.Contains(t, err.Error(), "download may be compromised")
	// The returned hash should be the hash of the fake content.
	assert.Contains(t, err.Error(), hashOf(fakeScript))
}

func TestInstallOhMyZsh_HTTPError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Serve a 500 so the status-code check fires.
	serveScript(t, nil, http.StatusInternalServerError)

	err := InstallOhMyZsh(false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), fmt.Sprintf("unexpected status %d", http.StatusInternalServerError))
}

func TestInstallOhMyZsh_DryRun_NoNetwork(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Dry-run must return without making any network call — URL is deliberately
	// set to something unreachable to catch any accidental HTTP access.
	orig := omzInstallURL
	omzInstallURL = "http://127.0.0.1:0/should-not-be-called"
	t.Cleanup(func() { omzInstallURL = orig })

	err := InstallOhMyZsh(true)
	assert.NoError(t, err)
}
