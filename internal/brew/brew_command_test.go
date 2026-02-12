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
