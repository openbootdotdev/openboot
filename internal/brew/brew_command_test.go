package brew

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupFakeBrew(t *testing.T, script string) {
	t.Helper()
	tmpDir := t.TempDir()
	brewPath := filepath.Join(tmpDir, "brew")
	require.NoError(t, os.WriteFile(brewPath, []byte(script), 0755))
	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", tmpDir+string(os.PathListSeparator)+originalPath)
}

func TestGetInstalledPackages_ParsesOutput(t *testing.T) {
	setupFakeBrew(t, "#!/bin/sh\n"+
		"if [ \"$1\" = \"list\" ] && [ \"$2\" = \"--formula\" ]; then\n"+
		"  echo git\n"+
		"  echo curl\n"+
		"  exit 0\n"+
		"fi\n"+
		"if [ \"$1\" = \"list\" ] && [ \"$2\" = \"--cask\" ]; then\n"+
		"  echo firefox\n"+
		"  exit 0\n"+
		"fi\n"+
		"exit 0\n")

	formulae, casks, err := GetInstalledPackages()
	require.NoError(t, err)
	assert.True(t, formulae["git"])
	assert.True(t, formulae["curl"])
	assert.True(t, casks["firefox"])
}

func TestListOutdated_ParsesJSON(t *testing.T) {
	setupFakeBrew(t, "#!/bin/sh\n"+
		"if [ \"$1\" = \"outdated\" ] && [ \"$2\" = \"--json\" ]; then\n"+
		"  cat <<'EOF'\n"+
		"{\"formulae\":[{\"name\":\"git\",\"installed_versions\":[\"2.0\"],\"current_version\":\"2.1\"}],\"casks\":[{\"name\":\"firefox\",\"installed_versions\":[\"1.0\"],\"current_version\":\"2.0\"}]}\n"+
		"EOF\n"+
		"  exit 0\n"+
		"fi\n"+
		"exit 0\n")

	outdated, err := ListOutdated()
	require.NoError(t, err)
	assert.Len(t, outdated, 2)
	assert.Equal(t, "git", outdated[0].Name)
	assert.Equal(t, "firefox (cask)", outdated[1].Name)
}

func TestDoctorDiagnose_Suggestions(t *testing.T) {
	setupFakeBrew(t, "#!/bin/sh\n"+
		"if [ \"$1\" = \"doctor\" ]; then\n"+
		"  echo 'Warning: unbrewed header files were found'\n"+
		"  echo 'Warning: broken symlinks detected'\n"+
		"  exit 0\n"+
		"fi\n"+
		"exit 0\n")

	suggestions, err := DoctorDiagnose()
	require.NoError(t, err)
	assert.Contains(t, suggestions, "Run: sudo rm -rf /usr/local/include")
	assert.Contains(t, suggestions, "Run: brew cleanup --prune=all")
}

func TestUpdateAndCleanup_UsesBrew(t *testing.T) {
	setupFakeBrew(t, "#!/bin/sh\n"+
		"if [ \"$1\" = \"update\" ]; then\n"+
		"  exit 0\n"+
		"fi\n"+
		"if [ \"$1\" = \"upgrade\" ]; then\n"+
		"  exit 0\n"+
		"fi\n"+
		"if [ \"$1\" = \"cleanup\" ]; then\n"+
		"  exit 0\n"+
		"fi\n"+
		"exit 0\n")

	err := Update(false)
	assert.NoError(t, err)

	err = Cleanup()
	assert.NoError(t, err)
}

func TestUninstall_Empty(t *testing.T) {
	err := Uninstall([]string{}, false)
	assert.NoError(t, err)
}

func TestUninstall_DryRun(t *testing.T) {
	err := Uninstall([]string{"git", "curl"}, true)
	assert.NoError(t, err)
}

func TestUninstall_Success(t *testing.T) {
	setupFakeBrew(t, "#!/bin/sh\nexit 0\n")
	err := Uninstall([]string{"wget", "jq"}, false)
	assert.NoError(t, err)
}

func TestUninstall_Failure(t *testing.T) {
	setupFakeBrew(t, "#!/bin/sh\necho 'Error: No such keg'\nexit 1\n")
	err := Uninstall([]string{"nonexistent"}, false)
	assert.Error(t, err)
}

func TestUninstall_PartialFailure(t *testing.T) {
	setupFakeBrew(t, "#!/bin/sh\n"+
		"if [ \"$2\" = \"bad-pkg\" ]; then\n"+
		"  echo 'Error: No such keg'\n"+
		"  exit 1\n"+
		"fi\n"+
		"exit 0\n")
	err := Uninstall([]string{"good-pkg", "bad-pkg"}, false)
	assert.Error(t, err)
}

func TestUninstallCask_Empty(t *testing.T) {
	err := UninstallCask([]string{}, false)
	assert.NoError(t, err)
}

func TestUninstallCask_DryRun(t *testing.T) {
	err := UninstallCask([]string{"firefox", "slack"}, true)
	assert.NoError(t, err)
}

func TestUninstallCask_Success(t *testing.T) {
	setupFakeBrew(t, "#!/bin/sh\nexit 0\n")
	err := UninstallCask([]string{"firefox"}, false)
	assert.NoError(t, err)
}

func TestUninstallCask_Failure(t *testing.T) {
	setupFakeBrew(t, "#!/bin/sh\necho 'Error: Cask not found'\nexit 1\n")
	err := UninstallCask([]string{"nonexistent-cask"}, false)
	assert.Error(t, err)
}

func TestDoctorDiagnose_ReadyToBrew(t *testing.T) {
	setupFakeBrew(t, "#!/bin/sh\nif [ \"$1\" = \"doctor\" ]; then\n  echo 'Your system is ready to brew.'\n  exit 0\nfi\nexit 0\n")
	suggestions, err := DoctorDiagnose()
	require.NoError(t, err)
	assert.Nil(t, suggestions)
}

func TestDoctorDiagnose_Failure(t *testing.T) {
	setupFakeBrew(t, "#!/bin/sh\nif [ \"$1\" = \"doctor\" ]; then\n  exit 1\nfi\nexit 0\n")
	_, err := DoctorDiagnose()
	assert.Error(t, err)
}

func TestDoctorDiagnose_MultipleWarnings(t *testing.T) {
	setupFakeBrew(t, "#!/bin/sh\n"+
		"if [ \"$1\" = \"doctor\" ]; then\n"+
		"  echo 'Warning: Unbrewed dylibs were found in /usr/local/lib'\n"+
		"  echo 'Warning: Your Homebrew/homebrew/core tap is not a full clone'\n"+
		"  echo 'Warning: Git origin remote mismatch'\n"+
		"  echo 'Warning: Uncommitted modifications to Homebrew'\n"+
		"  echo 'Warning: outdated Xcode command line tools'\n"+
		"  echo 'Warning: Broken symlinks were found'\n"+
		"  echo 'Warning: permission issues'\n"+
		"  exit 0\n"+
		"fi\n"+
		"exit 0\n")
	suggestions, err := DoctorDiagnose()
	require.NoError(t, err)
	assert.NotEmpty(t, suggestions)
	assert.Contains(t, suggestions, "Run: brew doctor --list-checks and review linked libraries")
	assert.Contains(t, suggestions, "Run: brew untap homebrew/core homebrew/cask")
	assert.Contains(t, suggestions, "Run: brew update-reset")
	assert.Contains(t, suggestions, "Run: xcode-select --install")
	assert.Contains(t, suggestions, "Run: brew cleanup --prune=all")
	assert.Contains(t, suggestions, "Run: sudo chown -R $(whoami) $(brew --prefix)/*")
}

func TestDoctorDiagnose_UnknownWarnings(t *testing.T) {
	setupFakeBrew(t, "#!/bin/sh\n"+
		"if [ \"$1\" = \"doctor\" ]; then\n"+
		"  echo 'Warning: Some unknown issue'\n"+
		"  exit 0\n"+
		"fi\n"+
		"exit 0\n")
	suggestions, err := DoctorDiagnose()
	require.NoError(t, err)
	assert.Contains(t, suggestions, "Run: brew doctor (to see full diagnostic output)")
}

func TestBrewInstallCmd_SetsNoAutoUpdate(t *testing.T) {
	cmd := brewInstallCmd("install", "git")
	assert.Equal(t, []string{"brew", "install", "git"}, cmd.Args)
	found := false
	for _, env := range cmd.Env {
		if env == "HOMEBREW_NO_AUTO_UPDATE=1" {
			found = true
		}
	}
	assert.True(t, found, "expected HOMEBREW_NO_AUTO_UPDATE=1 in env")
}
