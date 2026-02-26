package dotfiles

import (
	"os"
	"os/exec"
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

func TestBackupFile_CreatesBackup(t *testing.T) {
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "original")
	dst := filepath.Join(tmpDir, "backup")

	require.NoError(t, os.WriteFile(src, []byte("hello"), 0644))

	require.NoError(t, backupFile(src, dst))

	data, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, "hello", string(data))
}

func TestBackupFile_MissingSrcReturnsError(t *testing.T) {
	tmpDir := t.TempDir()
	err := backupFile(filepath.Join(tmpDir, "nonexistent"), filepath.Join(tmpDir, "backup"))
	assert.Error(t, err)
}

func TestRestoreFile_MovesBackToOriginal(t *testing.T) {
	tmpDir := t.TempDir()
	backup := filepath.Join(tmpDir, "file.bak")
	original := filepath.Join(tmpDir, "file")

	require.NoError(t, os.WriteFile(backup, []byte("restored"), 0644))

	restoreFile(backup, original)

	data, err := os.ReadFile(original)
	require.NoError(t, err)
	assert.Equal(t, "restored", string(data))

	_, err = os.Stat(backup)
	assert.True(t, os.IsNotExist(err))
}

func TestRestoreFile_NoopWhenBackupMissing(t *testing.T) {
	tmpDir := t.TempDir()
	restoreFile(filepath.Join(tmpDir, "nonexistent.bak"), filepath.Join(tmpDir, "original"))
}

// initBareAndClone creates a bare repo with one commit and clones it into
// ~/.dotfiles inside tmpHome. Returns the bare repo path.
func initBareAndClone(t *testing.T, tmpHome string) string {
	t.Helper()
	bare := filepath.Join(tmpHome, "remote.git")
	require.NoError(t, exec.Command("git", "init", "--bare", bare).Run())

	dotfilesPath := filepath.Join(tmpHome, defaultDotfilesDir)
	require.NoError(t, exec.Command("git", "clone", bare, dotfilesPath).Run())

	// Create an initial commit so the branch exists on origin.
	require.NoError(t, os.WriteFile(filepath.Join(dotfilesPath, ".bashrc"), []byte("# bashrc"), 0644))
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", dotfilesPath}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test", "GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test")
		require.NoError(t, cmd.Run())
	}
	run("add", ".")
	run("commit", "-m", "init")
	run("push")
	return bare
}

func TestClone_SyncExistingRepo(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	bare := initBareAndClone(t, tmpHome)
	dotfilesPath := filepath.Join(tmpHome, defaultDotfilesDir)

	// Push a new commit to the bare repo from a separate clone.
	scratch := filepath.Join(tmpHome, "scratch")
	require.NoError(t, exec.Command("git", "clone", bare, scratch).Run())
	require.NoError(t, os.WriteFile(filepath.Join(scratch, ".vimrc"), []byte("\" vimrc"), 0644))
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", scratch}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test", "GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test")
		require.NoError(t, cmd.Run())
	}
	run("add", ".")
	run("commit", "-m", "add vimrc")
	run("push")

	// Clone should fetch+reset and pick up the new file.
	err := Clone(bare, false)
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(dotfilesPath, ".vimrc"))
	assert.NoError(t, err, ".vimrc should exist after sync")
}

func TestClone_RemoteChangedBackupAndReclone(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	initBareAndClone(t, tmpHome)
	dotfilesPath := filepath.Join(tmpHome, defaultDotfilesDir)

	// Create a second bare repo (different remote).
	bare2 := filepath.Join(tmpHome, "remote2.git")
	require.NoError(t, exec.Command("git", "init", "--bare", bare2).Run())
	tmp2 := filepath.Join(tmpHome, "tmp2")
	require.NoError(t, exec.Command("git", "clone", bare2, tmp2).Run())
	require.NoError(t, os.WriteFile(filepath.Join(tmp2, ".zshrc"), []byte("# zshrc"), 0644))
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", tmp2}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test", "GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test")
		require.NoError(t, cmd.Run())
	}
	run("add", ".")
	run("commit", "-m", "init")
	run("push")

	// Clone with different URL should backup old and re-clone.
	err := Clone(bare2, false)
	require.NoError(t, err)

	// Old dotfiles should be backed up.
	_, err = os.Stat(dotfilesPath + ".openboot.bak")
	assert.NoError(t, err, "backup should exist")

	// New dotfiles should have .zshrc from the new remote.
	_, err = os.Stat(filepath.Join(dotfilesPath, ".zshrc"))
	assert.NoError(t, err, ".zshrc should exist from new remote")
}

func TestClone_BackupOverwriteOnRepeatedRemoteChange(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	bare1 := initBareAndClone(t, tmpHome)
	dotfilesPath := filepath.Join(tmpHome, defaultDotfilesDir)

	// Create two alternative bare repos.
	mkBare := func(name, file string) string {
		b := filepath.Join(tmpHome, name+".git")
		require.NoError(t, exec.Command("git", "init", "--bare", b).Run())
		tmp := filepath.Join(tmpHome, name+"-tmp")
		require.NoError(t, exec.Command("git", "clone", b, tmp).Run())
		require.NoError(t, os.WriteFile(filepath.Join(tmp, file), []byte("x"), 0644))
		cmd := exec.Command("git", "-C", tmp, "add", ".")
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test", "GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test")
		require.NoError(t, cmd.Run())
		cmd = exec.Command("git", "-C", tmp, "commit", "-m", "init")
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test", "GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test")
		require.NoError(t, cmd.Run())
		cmd = exec.Command("git", "-C", tmp, "push")
		require.NoError(t, cmd.Run())
		return b
	}
	bare2 := mkBare("remote2", ".file2")
	bare3 := mkBare("remote3", ".file3")
	_ = bare1

	// First remote change: bare1 → bare2
	require.NoError(t, Clone(bare2, false))
	_, err := os.Stat(dotfilesPath + ".openboot.bak")
	require.NoError(t, err, "first backup should exist")

	// Second remote change: bare2 → bare3 (should not fail due to existing backup)
	require.NoError(t, Clone(bare3, false))
	_, err = os.Stat(filepath.Join(dotfilesPath, ".file3"))
	assert.NoError(t, err, ".file3 should exist from third remote")
}

func TestClone_DetachedHeadFallback(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	bare := initBareAndClone(t, tmpHome)
	dotfilesPath := filepath.Join(tmpHome, defaultDotfilesDir)

	// Detach HEAD.
	require.NoError(t, exec.Command("git", "-C", dotfilesPath, "checkout", "--detach").Run())

	// Push a new commit from a scratch clone.
	scratch := filepath.Join(tmpHome, "scratch")
	require.NoError(t, exec.Command("git", "clone", bare, scratch).Run())
	require.NoError(t, os.WriteFile(filepath.Join(scratch, ".newfile"), []byte("x"), 0644))
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", scratch}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test", "GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test")
		require.NoError(t, cmd.Run())
	}
	run("add", ".")
	run("commit", "-m", "new")
	run("push")

	// Clone should handle detached HEAD and still sync.
	err := Clone(bare, false)
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(dotfilesPath, ".newfile"))
	assert.NoError(t, err, ".newfile should exist after sync from detached HEAD")
}

func TestLinkDirect_SkipsGitMetadata(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	dotfilesPath := filepath.Join(tmpHome, defaultDotfilesDir)
	require.NoError(t, os.MkdirAll(dotfilesPath, 0755))

	for _, name := range []string{".gitignore", ".gitmodules", ".gitattributes"} {
		require.NoError(t, os.WriteFile(filepath.Join(dotfilesPath, name), []byte("x"), 0644))
	}

	require.NoError(t, linkDirect(dotfilesPath, false))

	for _, name := range []string{".gitignore", ".gitmodules", ".gitattributes"} {
		_, err := os.Lstat(filepath.Join(tmpHome, name))
		assert.True(t, os.IsNotExist(err), "%s should not be linked", name)
	}
}

func TestLinkDirect_SkipsAlreadyCorrectSymlink(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	dotfilesPath := filepath.Join(tmpHome, defaultDotfilesDir)
	require.NoError(t, os.MkdirAll(dotfilesPath, 0755))

	src := filepath.Join(dotfilesPath, ".bashrc")
	dst := filepath.Join(tmpHome, ".bashrc")
	require.NoError(t, os.WriteFile(src, []byte("# bashrc"), 0644))

	// First link creates symlink + backup.
	require.NoError(t, linkDirect(dotfilesPath, false))
	target, err := os.Readlink(dst)
	require.NoError(t, err)
	assert.Equal(t, src, target)

	// Second link should not create a .openboot.bak (symlink is already correct).
	require.NoError(t, linkDirect(dotfilesPath, false))
	_, err = os.Stat(dst + ".openboot.bak")
	assert.True(t, os.IsNotExist(err), "backup should not exist when symlink is already correct")
}

func TestLinkWithStow_ZshBackupRestoredOnFailure(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	dotfilesPath := filepath.Join(tmpHome, defaultDotfilesDir)
	zshPkg := filepath.Join(dotfilesPath, "zsh")
	require.NoError(t, os.MkdirAll(zshPkg, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(zshPkg, ".zshrc"), []byte("zsh pkg zshrc"), 0644))

	zshrcPath := filepath.Join(tmpHome, ".zshrc")
	originalContent := "# original zshrc\n"
	require.NoError(t, os.WriteFile(zshrcPath, []byte(originalContent), 0644))

	err := linkWithStow(dotfilesPath, false)

	if err != nil {
		content, readErr := os.ReadFile(zshrcPath)
		require.NoError(t, readErr)
		assert.Equal(t, originalContent, string(content), ".zshrc should be restored after stow failure")
	}
}
