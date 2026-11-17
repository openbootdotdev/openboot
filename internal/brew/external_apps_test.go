package brew

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExternalApps(t *testing.T) {
	app := filepath.Join(t.TempDir(), "Custom Applications", "Typeless.app")
	require.NoError(t, os.MkdirAll(filepath.Join(app, "Contents"), 0755))
	plist := filepath.Join(app, "Contents", "Info.plist")
	require.NoError(t, os.WriteFile(plist, []byte("existing app data"), 0644))
	appArtifact := map[string]any{"app": []string{"Typeless.app"}, "target": app}
	for _, tc := range []struct {
		name      string
		artifacts []map[string]any
		installed any
		skip      bool
	}{
		{name: "external app", artifacts: []map[string]any{appArtifact}, skip: true},
		{name: "managed app", artifacts: []map[string]any{appArtifact}, installed: "2.12.0"},
		{name: "cleanup instructions", artifacts: []map[string]any{appArtifact, {"zap": []any{}}, {"uninstall": []any{}}}, skip: true},
		{name: "pkg installer", artifacts: []map[string]any{appArtifact, {"pkg": []string{"Installer.pkg"}}}},
		{name: "install hook", artifacts: []map[string]any{appArtifact, {"preflight": nil}}},
		{name: "helper binary", artifacts: []map[string]any{appArtifact, {"binary": []string{"helper"}}}},
		{name: "missing second app", artifacts: []map[string]any{appArtifact, {"app": []string{"Other.app"}, "target": filepath.Join(t.TempDir(), "Other.app")}}},
		{name: "missing resolved target", artifacts: []map[string]any{{"app": []string{"Typeless.app"}}}},
		{name: "relative target", artifacts: []map[string]any{{"app": []string{"Typeless.app"}, "target": "Typeless.app"}}},
		{name: "no apps", artifacts: []map[string]any{{"zap": []any{}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(map[string]any{"casks": []any{map[string]any{
				"token": "typeless", "full_token": "vendor/tap/typeless", "installed": tc.installed, "artifacts": tc.artifacts,
			}}})
			require.NoError(t, err)
			withFakeBrew(t, func(args []string) ([]byte, error) {
				assert.Equal(t, []string{"info", "--cask", "--json=v2", "vendor/tap/typeless", "other"}, args)
				return data, nil
			})
			pending := filterExternalApps([]string{"vendor/tap/typeless", "other"})
			if tc.skip {
				assert.Equal(t, []string{"other"}, pending)
			} else {
				assert.Equal(t, []string{"vendor/tap/typeless", "other"}, pending)
			}
			contents, err := os.ReadFile(plist)
			require.NoError(t, err)
			assert.Equal(t, "existing app data", string(contents))
		})
	}
}

func TestExternalApps_MetadataFailurePreservesInstall(t *testing.T) {
	for _, tc := range []struct {
		name string
		data string
		err  error
	}{
		{name: "brew failure", err: errors.New("offline")},
		{name: "malformed json", data: "{"},
		{name: "missing cask", data: `{"casks":[]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withFakeBrew(t, func([]string) ([]byte, error) { return []byte(tc.data), tc.err })
			assert.Equal(t, []string{"typeless"}, filterExternalApps([]string{"typeless"}))
		})
	}
}

func TestInstallWithProgress_ExternalAppNotRecordedOrInstalled(t *testing.T) {
	app := filepath.Join(t.TempDir(), "Typeless.app")
	require.NoError(t, os.MkdirAll(filepath.Join(app, "Contents"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(app, "Contents", "Info.plist"), []byte("preserve me"), 0644))
	data, err := json.Marshal(map[string]any{"casks": []any{map[string]any{
		"token": "typeless", "artifacts": []any{map[string]any{"app": []string{"Typeless.app"}, "target": app}},
	}}})
	require.NoError(t, err)
	withFakeBrew(t, func(args []string) ([]byte, error) {
		if args[0] == "list" {
			return nil, nil
		}
		require.Equal(t, []string{"info", "--cask", "--json=v2", "typeless"}, args)
		return data, nil
	})
	// Install commands bypass Runner. Remove brew from PATH so a regression
	// cannot modify the host or silently pass by installing the real app.
	t.Setenv("PATH", t.TempDir())
	output := captureOutput(t, func() {
		formulae, casks, err := InstallWithProgress(context.Background(), nil, []string{"typeless"}, false)
		require.NoError(t, err)
		assert.Empty(t, formulae)
		assert.Empty(t, casks, "external apps must not be persisted as Brew-managed")
	})
	assert.Contains(t, output, "Skipping typeless")
	assert.Contains(t, output, "not verified or managed by Homebrew")
	assert.Contains(t, output, "1 external apps skipped, 0 to install")
	assert.NotContains(t, strings.ToLower(output), "retrying")
}

func TestExternalApps_DryRunDoesNotInspectOrInstall(t *testing.T) {
	withFakeBrew(t, func([]string) ([]byte, error) {
		t.Fatal("dry run must not call brew")
		return nil, nil
	})
	_, _, err := InstallWithProgress(context.Background(), nil, []string{"typeless"}, true)
	require.NoError(t, err)
	require.NoError(t, InstallCask([]string{"typeless"}, true))
}

func TestInstallWithProgress_ExternalAppDoesNotBlockOtherCasks(t *testing.T) {
	root := t.TempDir()
	app := filepath.Join(root, "Typeless.app")
	require.NoError(t, os.MkdirAll(filepath.Join(app, "Contents"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(app, "Contents", "Info.plist"), []byte("existing app"), 0644))
	data, err := json.Marshal(map[string]any{"casks": []any{map[string]any{
		"token": "typeless", "artifacts": []any{map[string]any{"app": []string{"Typeless.app"}, "target": app}},
	}}})
	require.NoError(t, err)
	withFakeBrew(t, func(args []string) ([]byte, error) {
		if args[0] == "info" {
			return data, nil
		}
		return nil, nil
	})
	log := filepath.Join(root, "installs.log")
	setupFakeBrew(t, fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' \"$*\" >> %q\n", log))
	originalCheck := checkNetworkFunc
	checkNetworkFunc = func() error { return nil }
	t.Cleanup(func() { checkNetworkFunc = originalCheck })
	_, casks, err := InstallWithProgress(context.Background(), nil, []string{"typeless", "other"}, false)
	require.NoError(t, err)
	assert.Equal(t, []string{"other"}, casks)
	calls, err := os.ReadFile(log)
	require.NoError(t, err)
	assert.Equal(t, "install --cask other\n", string(calls))
}
