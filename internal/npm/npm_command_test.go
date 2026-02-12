package npm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupFakeNodeNpm(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	npmPath := filepath.Join(tmpDir, "npm")
	nodePath := filepath.Join(tmpDir, "node")

	npmScript := "#!/bin/sh\n" +
		"if [ \"$1\" = \"list\" ]; then\n" +
		"  echo /usr/local/lib/node_modules\n" +
		"  echo /usr/local/lib/node_modules/npm\n" +
		"  echo /usr/local/lib/node_modules/corepack\n" +
		"  echo /usr/local/lib/node_modules/typescript\n" +
		"  echo /usr/local/lib/node_modules/@scope/pkg\n" +
		"  exit 0\n" +
		"fi\n" +
		"if [ \"$1\" = \"install\" ]; then\n" +
		"  if [ -n \"$NPM_CALLS_FILE\" ]; then\n" +
		"    echo \"$@\" >> \"$NPM_CALLS_FILE\"\n" +
		"  fi\n" +
		"  echo installed\n" +
		"  exit 0\n" +
		"fi\n" +
		"exit 0\n"

	nodeScript := "#!/bin/sh\n" +
		"echo v22.1.0\n"

	require.NoError(t, os.WriteFile(npmPath, []byte(npmScript), 0755))
	require.NoError(t, os.WriteFile(nodePath, []byte(nodeScript), 0755))

	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", tmpDir+string(os.PathListSeparator)+originalPath)

	return tmpDir
}

func TestGetInstalledPackages_ParsesList(t *testing.T) {
	setupFakeNodeNpm(t)

	packages, err := GetInstalledPackages()
	require.NoError(t, err)
	assert.True(t, packages["typescript"])
	assert.True(t, packages["@scope/pkg"])
	assert.False(t, packages["npm"])
	assert.False(t, packages["corepack"])
}

func TestGetNodeVersion_ParsesVersion(t *testing.T) {
	setupFakeNodeNpm(t)

	version, err := GetNodeVersion()
	require.NoError(t, err)
	assert.Equal(t, 22, version)
}

func TestInstall_FiltersInstalledPackages(t *testing.T) {
	setupFakeNodeNpm(t)

	callsFile := filepath.Join(t.TempDir(), "npm_calls.txt")
	t.Setenv("NPM_CALLS_FILE", callsFile)

	err := Install([]string{"typescript", "eslint", "@scope/pkg"}, false)
	require.NoError(t, err)

	data, err := os.ReadFile(callsFile)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	require.GreaterOrEqual(t, len(lines), 1)
	assert.Contains(t, lines[0], "install -g eslint")
	assert.NotContains(t, lines[0], "typescript")
	assert.NotContains(t, lines[0], "@scope/pkg")
}
