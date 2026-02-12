package dotfiles

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetDotfilesURL_Empty(t *testing.T) {
	t.Setenv("OPENBOOT_DOTFILES", "")
	url := GetDotfilesURL()
	assert.Equal(t, "", url)
}

func TestGetDotfilesURL_Set(t *testing.T) {
	expected := "https://github.com/user/dotfiles"
	t.Setenv("OPENBOOT_DOTFILES", expected)
	url := GetDotfilesURL()
	assert.Equal(t, expected, url)
}

func TestClone_EmptyURL(t *testing.T) {
	err := Clone("", false)
	assert.NoError(t, err)
}

func TestClone_DryRun(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	err := Clone("https://github.com/user/dotfiles", true)
	assert.NoError(t, err)

	dotfilesPath := filepath.Join(tmpHome, defaultDotfilesDir)
	_, err = os.Stat(dotfilesPath)
	assert.True(t, os.IsNotExist(err))
}

func TestClone_AlreadyExists(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	dotfilesPath := filepath.Join(tmpHome, defaultDotfilesDir)
	err := os.MkdirAll(dotfilesPath, 0755)
	require.NoError(t, err)

	err = Clone("https://github.com/user/dotfiles", false)
	assert.NoError(t, err)
}

func TestLink_DotfilesDirNotExist(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	err := Link(false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestLink_DryRun(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	dotfilesPath := filepath.Join(tmpHome, defaultDotfilesDir)
	err := os.MkdirAll(dotfilesPath, 0755)
	require.NoError(t, err)

	testFile := filepath.Join(dotfilesPath, ".vimrc")
	err = os.WriteFile(testFile, []byte("test"), 0644)
	require.NoError(t, err)

	err = Link(true)
	assert.NoError(t, err)

	linkedFile := filepath.Join(tmpHome, ".vimrc")
	_, err = os.Lstat(linkedFile)
	assert.True(t, os.IsNotExist(err))
}

func TestHasStowPackages_Empty(t *testing.T) {
	tmpDir := t.TempDir()
	result := hasStowPackages(tmpDir)
	assert.False(t, result)
}

func TestHasStowPackages_NoDirs(t *testing.T) {
	tmpDir := t.TempDir()

	err := os.WriteFile(filepath.Join(tmpDir, "file.txt"), []byte("test"), 0644)
	require.NoError(t, err)

	result := hasStowPackages(tmpDir)
	assert.False(t, result)
}

func TestHasStowPackages_WithStowStructure(t *testing.T) {
	tmpDir := t.TempDir()

	pkgDir := filepath.Join(tmpDir, "vim")
	err := os.MkdirAll(pkgDir, 0755)
	require.NoError(t, err)

	dotfile := filepath.Join(pkgDir, ".vimrc")
	err = os.WriteFile(dotfile, []byte("test"), 0644)
	require.NoError(t, err)

	result := hasStowPackages(tmpDir)
	assert.True(t, result)
}

func TestHasStowPackages_HiddenDirIgnored(t *testing.T) {
	tmpDir := t.TempDir()

	hiddenDir := filepath.Join(tmpDir, ".git")
	err := os.MkdirAll(hiddenDir, 0755)
	require.NoError(t, err)

	result := hasStowPackages(tmpDir)
	assert.False(t, result)
}

func TestLinkDirect_DryRun(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	dotfilesPath := filepath.Join(tmpHome, defaultDotfilesDir)
	err := os.MkdirAll(dotfilesPath, 0755)
	require.NoError(t, err)

	testFile := filepath.Join(dotfilesPath, ".bashrc")
	err = os.WriteFile(testFile, []byte("test"), 0644)
	require.NoError(t, err)

	err = linkDirect(dotfilesPath, true)
	assert.NoError(t, err)

	linkedFile := filepath.Join(tmpHome, ".bashrc")
	_, err = os.Lstat(linkedFile)
	assert.True(t, os.IsNotExist(err))
}

func TestLinkDirect_SkipsGitDir(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	dotfilesPath := filepath.Join(tmpHome, defaultDotfilesDir)
	gitDir := filepath.Join(dotfilesPath, ".git")
	err := os.MkdirAll(gitDir, 0755)
	require.NoError(t, err)

	err = linkDirect(dotfilesPath, true)
	assert.NoError(t, err)
}

func TestLinkDirect_SkipsREADME(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	dotfilesPath := filepath.Join(tmpHome, defaultDotfilesDir)
	err := os.MkdirAll(dotfilesPath, 0755)
	require.NoError(t, err)

	readme := filepath.Join(dotfilesPath, "README.md")
	err = os.WriteFile(readme, []byte("test"), 0644)
	require.NoError(t, err)

	err = linkDirect(dotfilesPath, true)
	assert.NoError(t, err)
}

func TestLinkWithStow_DryRun(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	dotfilesPath := filepath.Join(tmpHome, defaultDotfilesDir)
	vimPkg := filepath.Join(dotfilesPath, "vim")
	err := os.MkdirAll(vimPkg, 0755)
	require.NoError(t, err)

	vimrc := filepath.Join(vimPkg, ".vimrc")
	err = os.WriteFile(vimrc, []byte("test"), 0644)
	require.NoError(t, err)

	err = linkWithStow(dotfilesPath, true)
	assert.NoError(t, err)
}

func TestLinkWithStow_SkipsHiddenDirs(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	dotfilesPath := filepath.Join(tmpHome, defaultDotfilesDir)
	hiddenDir := filepath.Join(dotfilesPath, ".hidden")
	err := os.MkdirAll(hiddenDir, 0755)
	require.NoError(t, err)

	err = linkWithStow(dotfilesPath, true)
	assert.NoError(t, err)
}

func TestLinkWithStow_SkipsFiles(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	dotfilesPath := filepath.Join(tmpHome, defaultDotfilesDir)
	err := os.MkdirAll(dotfilesPath, 0755)
	require.NoError(t, err)

	file := filepath.Join(dotfilesPath, "file.txt")
	err = os.WriteFile(file, []byte("test"), 0644)
	require.NoError(t, err)

	err = linkWithStow(dotfilesPath, true)
	assert.NoError(t, err)
}
