package config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestZshPluginRepoURL_KnownExternal(t *testing.T) {
	url, ok := ZshPluginRepoURL("zsh-autosuggestions")
	assert.True(t, ok, "zsh-autosuggestions should be a known external plugin")
	assert.True(t, strings.HasPrefix(url, "https://"), "repo URL must be https, got %q", url)
}

func TestZshPluginRepoURL_BuiltinReturnsFalse(t *testing.T) {
	for _, builtin := range []string{"git", "docker", "kubectl", "z"} {
		url, ok := ZshPluginRepoURL(builtin)
		assert.False(t, ok, "%s is a built-in OMZ plugin and must not be in the catalog", builtin)
		assert.Empty(t, url)
	}
}

func TestZshPluginRepoURL_UnknownReturnsFalse(t *testing.T) {
	url, ok := ZshPluginRepoURL("totally-made-up-plugin-xyz")
	assert.False(t, ok)
	assert.Empty(t, url)
}

func TestZshPluginRepoURL_AllCatalogEntriesAreHTTPS(t *testing.T) {
	entries := loadZshPlugins()
	assert.NotEmpty(t, entries, "catalog must not be empty")
	for _, e := range entries {
		assert.NotEmpty(t, e.Name, "every entry needs a name")
		assert.True(t, strings.HasPrefix(e.Repo, "https://"), "%s repo must be https, got %q", e.Name, e.Repo)
	}
}
