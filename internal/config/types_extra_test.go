package config

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- PackageEntryList.Names ----

func TestPackageEntryList_Names_Empty(t *testing.T) {
	var list PackageEntryList
	names := list.Names()
	assert.Equal(t, []string{}, names)
}

func TestPackageEntryList_Names_SingleEntry(t *testing.T) {
	list := PackageEntryList{{Name: "git"}}
	assert.Equal(t, []string{"git"}, list.Names())
}

func TestPackageEntryList_Names_MultipleEntries(t *testing.T) {
	list := PackageEntryList{
		{Name: "git", Desc: "VCS"},
		{Name: "curl", Desc: "HTTP client"},
		{Name: "jq"},
	}
	assert.Equal(t, []string{"git", "curl", "jq"}, list.Names())
}

func TestPackageEntryList_Names_PreservesOrder(t *testing.T) {
	list := PackageEntryList{
		{Name: "z"},
		{Name: "a"},
		{Name: "m"},
	}
	names := list.Names()
	require.Len(t, names, 3)
	assert.Equal(t, "z", names[0])
	assert.Equal(t, "a", names[1])
	assert.Equal(t, "m", names[2])
}

// ---- PackageEntryList.DescMap ----

func TestPackageEntryList_DescMap_Empty(t *testing.T) {
	var list PackageEntryList
	m := list.DescMap()
	assert.NotNil(t, m)
	assert.Empty(t, m)
}

func TestPackageEntryList_DescMap_NoDescriptions(t *testing.T) {
	list := PackageEntryList{
		{Name: "git"},
		{Name: "curl"},
	}
	m := list.DescMap()
	assert.Empty(t, m)
}

func TestPackageEntryList_DescMap_SomeDescriptions(t *testing.T) {
	list := PackageEntryList{
		{Name: "git", Desc: "Version control"},
		{Name: "curl"},
		{Name: "jq", Desc: "JSON processor"},
	}
	m := list.DescMap()
	assert.Equal(t, map[string]string{"git": "Version control", "jq": "JSON processor"}, m)
}

func TestPackageEntryList_DescMap_AllDescriptions(t *testing.T) {
	list := PackageEntryList{
		{Name: "git", Desc: "Version control"},
		{Name: "curl", Desc: "HTTP client"},
	}
	m := list.DescMap()
	assert.Equal(t, "Version control", m["git"])
	assert.Equal(t, "HTTP client", m["curl"])
}

// ---- PackageEntryList.UnmarshalJSON ----

func TestPackageEntryList_UnmarshalJSON_FlatStrings(t *testing.T) {
	data := []byte(`["git","curl","jq"]`)
	var list PackageEntryList
	require.NoError(t, json.Unmarshal(data, &list))
	assert.Equal(t, PackageEntryList{{Name: "git"}, {Name: "curl"}, {Name: "jq"}}, list)
}

func TestPackageEntryList_UnmarshalJSON_ObjectArray(t *testing.T) {
	data := []byte(`[{"name":"git","desc":"VCS"},{"name":"curl"}]`)
	var list PackageEntryList
	require.NoError(t, json.Unmarshal(data, &list))
	assert.Equal(t, "git", list[0].Name)
	assert.Equal(t, "VCS", list[0].Desc)
	assert.Equal(t, "curl", list[1].Name)
	assert.Equal(t, "", list[1].Desc)
}

func TestPackageEntryList_UnmarshalJSON_EmptyArray(t *testing.T) {
	data := []byte(`[]`)
	var list PackageEntryList
	require.NoError(t, json.Unmarshal(data, &list))
	assert.Empty(t, list)
}

func TestPackageEntryList_UnmarshalJSON_InvalidJSON(t *testing.T) {
	data := []byte(`not json`)
	var list PackageEntryList
	err := json.Unmarshal(data, &list)
	assert.Error(t, err)
}

// ---- RemoteConfig struct validation helpers ----

func TestRemoteConfig_EmptyIsValid(t *testing.T) {
	rc := &RemoteConfig{}
	err := rc.Validate()
	assert.NoError(t, err)
}

func TestRemoteConfig_WithValidPackages(t *testing.T) {
	rc := &RemoteConfig{
		Packages: PackageEntryList{{Name: "git"}, {Name: "curl"}},
		Casks:    PackageEntryList{{Name: "firefox"}},
		Taps:     []string{"homebrew/core"},
		Npm:      PackageEntryList{{Name: "typescript"}},
	}
	err := rc.Validate()
	assert.NoError(t, err)
}

// Homebrew casks can contain '+' (e.g. logi-options+, gtk+). See issue #101.
func TestRemoteConfig_AllowsPlusInPackageNames(t *testing.T) {
	rc := &RemoteConfig{
		Packages: PackageEntryList{{Name: "gtk+"}},
		Casks:    PackageEntryList{{Name: "logi-options+"}},
	}
	err := rc.Validate()
	assert.NoError(t, err)
}

func TestRemoteConfig_InvalidPackageName(t *testing.T) {
	rc := &RemoteConfig{
		Packages: PackageEntryList{{Name: "bad name with spaces"}},
	}
	err := rc.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid package name")
}

func TestRemoteConfig_InvalidCaskName(t *testing.T) {
	rc := &RemoteConfig{
		Casks: PackageEntryList{{Name: "bad name!"}},
	}
	err := rc.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid cask name")
}

func TestRemoteConfig_InvalidNpmName(t *testing.T) {
	rc := &RemoteConfig{
		Npm: PackageEntryList{{Name: "bad name!"}},
	}
	err := rc.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid npm package name")
}

func TestRemoteConfig_InvalidTapName(t *testing.T) {
	rc := &RemoteConfig{
		Taps: []string{"notvalidtap"},
	}
	err := rc.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid tap name")
}

// ---- SnapshotGitConfig ----

func TestSnapshotGitConfig_Fields(t *testing.T) {
	g := &SnapshotGitConfig{UserName: "Alice", UserEmail: "alice@example.com"}
	assert.Equal(t, "Alice", g.UserName)
	assert.Equal(t, "alice@example.com", g.UserEmail)
}

// ---- RemoteShellConfig ----

func TestRemoteShellConfig_Fields(t *testing.T) {
	s := &RemoteShellConfig{OhMyZsh: true, Theme: "robbyrussell", Plugins: []string{"git", "z"}}
	assert.True(t, s.OhMyZsh)
	assert.Equal(t, "robbyrussell", s.Theme)
	assert.Equal(t, []string{"git", "z"}, s.Plugins)
}

// ---- RemoteMacOSPref ----

func TestRemoteMacOSPref_Fields(t *testing.T) {
	p := RemoteMacOSPref{
		Domain: "com.apple.dock",
		Key:    "autohide",
		Type:   "bool",
		Value:  "true",
		Desc:   "Auto-hide Dock",
	}
	assert.Equal(t, "com.apple.dock", p.Domain)
	assert.Equal(t, "autohide", p.Key)
	assert.Equal(t, "bool", p.Type)
	assert.Equal(t, "true", p.Value)
	assert.Equal(t, "Auto-hide Dock", p.Desc)
}

// ---- RemoteConfig DockApps and LoginItems ----

func TestRemoteConfig_DecodeDockAndLoginItems(t *testing.T) {
	raw := []byte(`{
	  "username":"alice","slug":"dev","name":"Dev","preset":"developer",
	  "packages":[],"casks":[],"taps":[],"npm":[],
	  "dotfiles_repo":"","post_install":[],
	  "macos_prefs":[],
	  "dock_apps":["/Applications/Zed.app","/Applications/Chrome.app"],
	  "login_items":[
	    {"name":"Maccy","path":"/Applications/Maccy.app"},
	    {"name":"BetterDisplay","path":"/Applications/BetterDisplay.app","hidden":true}
	  ]
	}`)
	var rc RemoteConfig
	require.NoError(t, json.Unmarshal(raw, &rc))
	assert.Equal(t, []string{"/Applications/Zed.app", "/Applications/Chrome.app"}, rc.DockApps)
	require.Len(t, rc.LoginItems, 2)
	assert.Equal(t, "Maccy", rc.LoginItems[0].Name)
	assert.False(t, rc.LoginItems[0].Hidden)
	assert.True(t, rc.LoginItems[1].Hidden)
}

func TestRemoteConfig_OldConfigOmittingNewFieldsStillDecodes(t *testing.T) {
	raw := []byte(`{"username":"a","slug":"b","name":"c","preset":"developer",
	  "packages":[],"casks":[],"taps":[],"npm":[],"dotfiles_repo":"",
	  "post_install":[],"macos_prefs":[]}`)
	var rc RemoteConfig
	require.NoError(t, json.Unmarshal(raw, &rc))
	assert.Nil(t, rc.DockApps)
	assert.Nil(t, rc.LoginItems)
}
