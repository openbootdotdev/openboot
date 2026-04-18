package npm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeRunner routes npm invocations through a Go handler, avoiding fork/exec.
type fakeRunner struct {
	handler func(args []string) ([]byte, error)
}

func (f *fakeRunner) Output(args ...string) ([]byte, error) {
	return f.handler(args)
}

func (f *fakeRunner) CombinedOutput(args ...string) ([]byte, error) {
	return f.handler(args)
}

func withFakeNpm(t *testing.T, handler func(args []string) ([]byte, error)) {
	t.Helper()
	t.Cleanup(SetRunner(&fakeRunner{handler: handler}))
}

func TestGetInstalledPackages_ParsesList(t *testing.T) {
	withFakeNpm(t, func(args []string) ([]byte, error) {
		if len(args) > 0 && args[0] == "list" {
			// npm list -g --parseable output: paths, one per line.
			return []byte(`/usr/local/lib/node_modules
/usr/local/lib/node_modules/npm
/usr/local/lib/node_modules/corepack
/usr/local/lib/node_modules/typescript
/usr/local/lib/node_modules/@scope/pkg
`), nil
		}
		return nil, nil
	})

	packages, err := GetInstalledPackages()
	require.NoError(t, err)
	assert.True(t, packages["typescript"])
	assert.True(t, packages["@scope/pkg"])
	assert.False(t, packages["npm"])
	assert.False(t, packages["corepack"])
}

func TestGetNodeVersion_ParsesVersion(t *testing.T) {
	orig := nodeVersionOutput
	t.Cleanup(func() { nodeVersionOutput = orig })
	nodeVersionOutput = func() ([]byte, error) { return []byte("v22.1.0\n"), nil }

	version, err := GetNodeVersion()
	require.NoError(t, err)
	assert.Equal(t, 22, version)
}

func TestInstall_FiltersInstalledPackages(t *testing.T) {
	var installCalls [][]string
	withFakeNpm(t, func(args []string) ([]byte, error) {
		if len(args) > 0 && args[0] == "list" {
			// typescript and @scope/pkg already installed; eslint is not.
			return []byte(`/usr/local/lib/node_modules
/usr/local/lib/node_modules/typescript
/usr/local/lib/node_modules/@scope/pkg
`), nil
		}
		if len(args) > 0 && args[0] == "install" {
			installCalls = append(installCalls, append([]string(nil), args...))
			return []byte("installed"), nil
		}
		return nil, nil
	})

	err := Install([]string{"typescript", "eslint", "@scope/pkg"}, false)
	require.NoError(t, err)

	require.Len(t, installCalls, 1, "expected one batch install call")
	call := installCalls[0]
	// Should install only eslint (other two are already present).
	assert.Contains(t, call, "eslint")
	assert.NotContains(t, call, "typescript")
	assert.NotContains(t, call, "@scope/pkg")
}
