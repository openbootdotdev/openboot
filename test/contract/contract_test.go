//go:build contract

package contract_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openbootdotdev/openboot/internal/config"
	"github.com/openbootdotdev/openboot/internal/snapshot"
)

type canonicalPackageEntry struct {
	Name string `json:"name"`
	Desc string `json:"desc"`
}

type canonicalRemoteConfig struct {
	Username     string                    `json:"username"`
	Slug         string                    `json:"slug"`
	Name         string                    `json:"name"`
	Preset       string                    `json:"preset"`
	Packages     []canonicalPackageEntry   `json:"packages"`
	Casks        []canonicalPackageEntry   `json:"casks"`
	Taps         []string                  `json:"taps"`
	Npm          []canonicalPackageEntry   `json:"npm"`
	DotfilesRepo string                    `json:"dotfiles_repo"`
	PostInstall  []string                  `json:"post_install"`
	Shell        *config.RemoteShellConfig `json:"shell"`
	MacOSPrefs   []config.RemoteMacOSPref  `json:"macos_prefs"`
}

func TestRemoteConfigFixtureIsConsumedLosslessly(t *testing.T) {
	data := readContractFixture(t, "config-v1.json")

	var wire canonicalRemoteConfig
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	require.NoError(t, decoder.Decode(&wire), "contract fixture must use the canonical remote-config shape")
	require.NotEmpty(t, wire.Packages, "fixture must exercise formulae")
	require.NotEmpty(t, wire.Casks, "fixture must exercise casks")
	require.NotEmpty(t, wire.Taps, "fixture must exercise taps")
	require.NotEmpty(t, wire.Npm, "fixture must exercise npm packages")

	got, err := config.UnmarshalRemoteConfigFlexible(data)
	require.NoError(t, err)
	require.NoError(t, got.Validate())

	want := &config.RemoteConfig{
		Username:     wire.Username,
		Slug:         wire.Slug,
		Name:         wire.Name,
		Preset:       wire.Preset,
		Packages:     packageEntries(wire.Packages),
		Casks:        packageEntries(wire.Casks),
		Taps:         wire.Taps,
		Npm:          packageEntries(wire.Npm),
		DotfilesRepo: wire.DotfilesRepo,
		PostInstall:  wire.PostInstall,
		Shell:        wire.Shell,
		MacOSPrefs:   wire.MacOSPrefs,
	}
	assert.Equal(t, want, got, "CLI decoding must not repair, move, or drop fields from the canonical fixture")
}

func TestSnapshotFixtureIsConsumedLosslessly(t *testing.T) {
	data := readContractFixture(t, "snapshot-v1.json")

	var wire struct {
		Packages struct {
			Formulae []string `json:"formulae"`
			Casks    []string `json:"casks"`
			Taps     []string `json:"taps"`
			Npm      []string `json:"npm"`
		} `json:"packages"`
	}
	require.NoError(t, json.Unmarshal(data, &wire))
	require.NotEmpty(t, wire.Packages.Formulae, "fixture must exercise formulae")
	require.NotEmpty(t, wire.Packages.Casks, "fixture must exercise casks")
	require.NotEmpty(t, wire.Packages.Taps, "fixture must exercise taps")
	require.NotEmpty(t, wire.Packages.Npm, "fixture must exercise npm packages")

	got, err := snapshot.ParseBytes(data)
	require.NoError(t, err)
	assert.Equal(t, wire.Packages.Formulae, got.Packages.Formulae)
	assert.Equal(t, wire.Packages.Casks, got.Packages.Casks)
	assert.Equal(t, wire.Packages.Taps, got.Packages.Taps)
	assert.Equal(t, wire.Packages.Npm, got.Packages.Npm)
}

func readContractFixture(t *testing.T, name string) []byte {
	t.Helper()

	contractDir := os.Getenv("OPENBOOT_CONTRACT_DIR")
	require.NotEmpty(t, contractDir, "OPENBOOT_CONTRACT_DIR must point to an openboot-contract checkout")

	data, err := os.ReadFile(filepath.Join(contractDir, "fixtures", name))
	require.NoError(t, err)
	return data
}

func packageEntries(entries []canonicalPackageEntry) config.PackageEntryList {
	result := make(config.PackageEntryList, len(entries))
	for i, entry := range entries {
		result[i] = config.PackageEntry{Name: entry.Name, Desc: entry.Desc}
	}
	return result
}
