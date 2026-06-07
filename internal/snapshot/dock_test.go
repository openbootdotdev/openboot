package snapshot

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseDockAppsJSON_Empty(t *testing.T) {
	got, err := parseDockAppsJSON([]byte(`[]`))
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestParseDockAppsJSON_TwoApps(t *testing.T) {
	input := []byte(`[
	  {"tile-data":{"file-data":{"_CFURLString":"file:///Applications/Google%20Chrome.app/","_CFURLStringType":15}},"tile-type":"file-tile"},
	  {"tile-data":{"file-data":{"_CFURLString":"file:///Applications/Zed.app/","_CFURLStringType":15}},"tile-type":"file-tile"}
	]`)
	got, err := parseDockAppsJSON(input)
	require.NoError(t, err)
	assert.Equal(t, []string{
		"/Applications/Google Chrome.app",
		"/Applications/Zed.app",
	}, got)
}

func TestParseDockAppsJSON_NonAppTileSkipped(t *testing.T) {
	input := []byte(`[
	  {"tile-data":{"file-data":{"_CFURLString":"file:///Applications/Zed.app/","_CFURLStringType":15}},"tile-type":"file-tile"},
	  {"tile-data":{"arrangement":2,"displayas":1},"tile-type":"directory-tile"}
	]`)
	got, err := parseDockAppsJSON(input)
	require.NoError(t, err)
	assert.Equal(t, []string{"/Applications/Zed.app"}, got)
}

func TestParseDockAppsJSON_NonASCIIPath(t *testing.T) {
	input := []byte(`[
	  {"tile-data":{"file-data":{"_CFURLString":"file:///Applications/%E5%BE%AE%E4%BF%A1.app/","_CFURLStringType":15}},"tile-type":"file-tile"}
	]`)
	got, err := parseDockAppsJSON(input)
	require.NoError(t, err)
	assert.Equal(t, []string{"/Applications/微信.app"}, got)
}

func TestParseDockAppsJSON_MalformedJSON(t *testing.T) {
	_, err := parseDockAppsJSON([]byte(`not json`))
	assert.Error(t, err)
}
