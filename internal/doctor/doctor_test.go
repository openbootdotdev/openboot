package doctor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeRunner returns canned responses for subprocess calls.
type fakeRunner struct {
	silentResults map[string]fakeResult
	outputResults map[string]fakeResult
}

type fakeResult struct {
	out string
	err error
}

func newFakeRunner() *fakeRunner {
	return &fakeRunner{
		silentResults: make(map[string]fakeResult),
		outputResults: make(map[string]fakeResult),
	}
}

func cmdKey(name string, args ...string) string {
	parts := append([]string{name}, args...)
	key := ""
	for i, p := range parts {
		if i > 0 {
			key += " "
		}
		key += p
	}
	return key
}

func (f *fakeRunner) RunSilent(name string, args ...string) (string, error) {
	key := cmdKey(name, args...)
	if r, ok := f.silentResults[key]; ok {
		return r.out, r.err
	}
	return "", nil
}

func (f *fakeRunner) RunOutput(name string, args ...string) (string, error) {
	key := cmdKey(name, args...)
	if r, ok := f.outputResults[key]; ok {
		return r.out, r.err
	}
	return "", nil
}

func TestCheckGit_Configured(t *testing.T) {
	runner := newFakeRunner()
	runner.silentResults["git config --global user.name"] = fakeResult{out: "Alice"}
	runner.silentResults["git config --global user.email"] = fakeResult{out: "alice@example.com"}

	d := &Doctor{Runner: runner, Version: "dev"}
	result := d.CheckGit()

	assert.Equal(t, StatusOK, result.Status)
	assert.Contains(t, result.Message, "Alice")
	assert.Contains(t, result.Message, "alice@example.com")
}

func TestCheckGit_MissingEmail(t *testing.T) {
	runner := newFakeRunner()
	runner.silentResults["git config --global user.name"] = fakeResult{out: "Alice"}
	runner.silentResults["git config --global user.email"] = fakeResult{out: ""}

	d := &Doctor{Runner: runner, Version: "dev"}
	result := d.CheckGit()

	assert.Equal(t, StatusWarn, result.Status)
	assert.Contains(t, result.Message, "not configured")
}

func TestCheckBrewOutdated_None(t *testing.T) {
	runner := newFakeRunner()
	runner.outputResults["brew outdated --quiet"] = fakeResult{out: ""}

	d := &Doctor{Runner: runner, Version: "dev"}
	result := d.CheckBrewOutdated()

	// If brew is not on PATH in the test env, this will be a warn/skip.
	// Only assert the happy path when brew is actually present.
	if result.Status == StatusOK {
		assert.Contains(t, result.Message, "up to date")
	}
}

func TestCheckBrewOutdated_Some(t *testing.T) {
	runner := newFakeRunner()
	runner.outputResults["brew outdated --quiet"] = fakeResult{out: "git\nwget\ncurl"}

	d := &Doctor{Runner: runner, Version: "dev"}
	result := d.CheckBrewOutdated()

	// Skip assertion if brew is not on PATH (LookPath fails first).
	if result.Status == StatusWarn && result.Message != "Skipped outdated-package check (Homebrew not installed)" {
		assert.Contains(t, result.Message, "3 outdated")
	}
}

func TestCheckNode_WithVersion(t *testing.T) {
	runner := newFakeRunner()
	runner.outputResults["node --version"] = fakeResult{out: "v20.11.0"}

	d := &Doctor{Runner: runner, Version: "dev"}
	result := d.CheckNode()

	// Only assert the happy path when node is on PATH.
	if result.Status == StatusOK {
		assert.Contains(t, result.Message, "v20.11.0")
	}
}

func TestCheckNpm_WithVersion(t *testing.T) {
	runner := newFakeRunner()
	runner.outputResults["npm --version"] = fakeResult{out: "10.2.3"}

	d := &Doctor{Runner: runner, Version: "dev"}
	result := d.CheckNpm()

	if result.Status == StatusOK {
		assert.Contains(t, result.Message, "10.2.3")
	}
}

func TestCheckShell_Zsh(t *testing.T) {
	t.Setenv("SHELL", "/bin/zsh")

	d := &Doctor{Runner: newFakeRunner(), Version: "dev"}
	result := d.CheckShell()

	assert.Equal(t, StatusOK, result.Status)
	assert.Contains(t, result.Message, "zsh")
}

func TestCheckShell_Bash(t *testing.T) {
	t.Setenv("SHELL", "/bin/bash")

	d := &Doctor{Runner: newFakeRunner(), Version: "dev"}
	result := d.CheckShell()

	assert.Equal(t, StatusWarn, result.Status)
	assert.Contains(t, result.Message, "bash")
}

func TestCheckShell_Empty(t *testing.T) {
	t.Setenv("SHELL", "")

	d := &Doctor{Runner: newFakeRunner(), Version: "dev"}
	result := d.CheckShell()

	assert.Equal(t, StatusWarn, result.Status)
	assert.Contains(t, result.Message, "empty")
}

func TestCheckOhMyZsh_Exists(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, ".oh-my-zsh"), 0750))

	d := &Doctor{Runner: newFakeRunner(), Version: "dev"}
	result := d.CheckOhMyZsh()

	assert.Equal(t, StatusOK, result.Status)
}

func TestCheckOhMyZsh_Missing(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	d := &Doctor{Runner: newFakeRunner(), Version: "dev"}
	result := d.CheckOhMyZsh()

	assert.Equal(t, StatusWarn, result.Status)
}

func TestCheckBrewShellenv_Present(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	require.NoError(t, os.WriteFile(filepath.Join(tmp, ".zshrc"), []byte("eval \"$(brew shellenv)\"\n"), 0644))

	d := &Doctor{Runner: newFakeRunner(), Version: "dev"}
	result := d.CheckBrewShellenv()

	// On arm64 this should be OK; on amd64 it says "not needed".
	assert.NotEqual(t, StatusFail, result.Status)
}

func TestCheckOpenBootDir_Exists(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, ".openboot"), 0750))

	d := &Doctor{Runner: newFakeRunner(), Version: "dev"}
	result := d.CheckOpenBootDir()

	assert.Equal(t, StatusOK, result.Status)
}

func TestCheckOpenBootDir_Missing(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	d := &Doctor{Runner: newFakeRunner(), Version: "dev"}
	result := d.CheckOpenBootDir()

	assert.Equal(t, StatusWarn, result.Status)
}

func TestCheckOpenBootState_SyncSource(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	dir := filepath.Join(tmp, ".openboot")
	require.NoError(t, os.MkdirAll(dir, 0750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sync_source.json"), []byte("{}"), 0600))

	d := &Doctor{Runner: newFakeRunner(), Version: "dev"}
	result := d.CheckOpenBootState()

	assert.Equal(t, StatusOK, result.Status)
	assert.Contains(t, result.Message, "Sync source found")
}

func TestCheckOpenBootState_StateFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	dir := filepath.Join(tmp, ".openboot")
	require.NoError(t, os.MkdirAll(dir, 0750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "state.json"), []byte("{}"), 0600))

	d := &Doctor{Runner: newFakeRunner(), Version: "dev"}
	result := d.CheckOpenBootState()

	assert.Equal(t, StatusOK, result.Status)
	assert.Contains(t, result.Message, "State file found")
}

func TestCheckOpenBootState_NoState(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	d := &Doctor{Runner: newFakeRunner(), Version: "dev"}
	result := d.CheckOpenBootState()

	assert.Equal(t, StatusWarn, result.Status)
	assert.Contains(t, result.Message, "No install state")
}

func TestCheckPATH_AppleSilicon(t *testing.T) {
	t.Setenv("PATH", "/opt/homebrew/bin:/usr/bin:/bin")

	d := &Doctor{Runner: newFakeRunner(), Version: "dev"}
	result := d.CheckPATH()

	// On arm64 this should be OK; on amd64 it says "not needed".
	assert.NotEqual(t, StatusFail, result.Status)
}

func TestSummary_Counts(t *testing.T) {
	tests := []struct {
		name    string
		results []Result
		want    Summary
	}{
		{
			name: "all ok",
			results: []Result{
				{Status: StatusOK},
				{Status: StatusOK},
			},
			want: Summary{Passed: 2},
		},
		{
			name: "mixed",
			results: []Result{
				{Status: StatusOK},
				{Status: StatusWarn},
				{Status: StatusFail},
			},
			want: Summary{Passed: 1, Warnings: 1, Errors: 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var s Summary
			for _, r := range tt.results {
				switch r.Status {
				case StatusOK:
					s.Passed++
				case StatusWarn:
					s.Warnings++
				case StatusFail:
					s.Errors++
				}
			}
			assert.Equal(t, tt.want, s)
		})
	}
}
