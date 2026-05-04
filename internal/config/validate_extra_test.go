package config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- validateMacOSPrefs — previously uncovered branches ----

func TestValidateMacOSPrefs_ValidTypes(t *testing.T) {
	validTypes := []string{"", "string", "int", "bool", "float"}
	for _, typ := range validTypes {
		t.Run("type="+typ, func(t *testing.T) {
			rc := &RemoteConfig{
				MacOSPrefs: []RemoteMacOSPref{
					{Domain: "com.apple.dock", Key: "autohide", Type: typ, Value: "true"},
				},
			}
			err := validateMacOSPrefs(rc)
			assert.NoError(t, err)
		})
	}
}

func TestValidateMacOSPrefs_InvalidType(t *testing.T) {
	rc := &RemoteConfig{
		MacOSPrefs: []RemoteMacOSPref{
			{Domain: "com.apple.dock", Key: "autohide", Type: "boolean"},
		},
	}
	err := validateMacOSPrefs(rc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid macos_prefs type")
	assert.Contains(t, err.Error(), "boolean")
}

func TestValidateMacOSPrefs_DomainStartsWithDash(t *testing.T) {
	rc := &RemoteConfig{
		MacOSPrefs: []RemoteMacOSPref{
			{Domain: "-com.apple.dock", Key: "autohide", Type: "bool"},
		},
	}
	err := validateMacOSPrefs(rc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must not start with '-'")
}

func TestValidateMacOSPrefs_KeyStartsWithDash(t *testing.T) {
	rc := &RemoteConfig{
		MacOSPrefs: []RemoteMacOSPref{
			{Domain: "com.apple.dock", Key: "-autohide", Type: "bool"},
		},
	}
	err := validateMacOSPrefs(rc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must not start with '-'")
}

func TestValidateMacOSPrefs_InvalidDomainCharacters(t *testing.T) {
	rc := &RemoteConfig{
		MacOSPrefs: []RemoteMacOSPref{
			{Domain: "com.apple;dock", Key: "autohide", Type: "bool"},
		},
	}
	err := validateMacOSPrefs(rc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid characters")
}

func TestValidateMacOSPrefs_InvalidKeyCharacters(t *testing.T) {
	rc := &RemoteConfig{
		MacOSPrefs: []RemoteMacOSPref{
			{Domain: "com.apple.dock", Key: "auto!hide", Type: "bool"},
		},
	}
	err := validateMacOSPrefs(rc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid characters")
}

func TestValidateMacOSPrefs_EmptyPrefs(t *testing.T) {
	rc := &RemoteConfig{MacOSPrefs: []RemoteMacOSPref{}}
	err := validateMacOSPrefs(rc)
	assert.NoError(t, err)
}

func TestValidateMacOSPrefs_NilPrefs(t *testing.T) {
	rc := &RemoteConfig{MacOSPrefs: nil}
	err := validateMacOSPrefs(rc)
	assert.NoError(t, err)
}

func TestValidateMacOSPrefs_MultiplePrefs_AllValid(t *testing.T) {
	rc := &RemoteConfig{
		MacOSPrefs: []RemoteMacOSPref{
			{Domain: "com.apple.dock", Key: "autohide", Type: "bool", Value: "true"},
			{Domain: "NSGlobalDomain", Key: "AppleShowScrollBars", Type: "string", Value: "Always"},
			{Domain: "com.apple.finder", Key: "ShowPathbar", Type: "bool", Value: "true"},
		},
	}
	err := validateMacOSPrefs(rc)
	assert.NoError(t, err)
}

func TestValidateMacOSPrefs_MultiplePrefs_SecondInvalid(t *testing.T) {
	rc := &RemoteConfig{
		MacOSPrefs: []RemoteMacOSPref{
			{Domain: "com.apple.dock", Key: "autohide", Type: "bool"},
			{Domain: "com.apple.dock", Key: "bad$key", Type: "bool"},
		},
	}
	err := validateMacOSPrefs(rc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid characters")
}

func TestValidateMacOSPrefs_KeyWithSpaces(t *testing.T) {
	rc := &RemoteConfig{
		MacOSPrefs: []RemoteMacOSPref{
			{Domain: "com.apple.systemuiserver", Key: "NSStatusItem Visible Sound", Type: "bool", Value: "true"},
		},
	}
	err := validateMacOSPrefs(rc)
	assert.NoError(t, err)
}

func TestValidateMacOSPrefs_DomainWithSpecialValidChars(t *testing.T) {
	// Domain and key regex: [a-zA-Z0-9 ._-]+
	validDomains := []string{
		"com.apple.dock",
		"NSGlobalDomain",
		"com.example-app.settings",
		"com.example_app.v2",
	}
	for _, domain := range validDomains {
		t.Run(domain, func(t *testing.T) {
			rc := &RemoteConfig{
				MacOSPrefs: []RemoteMacOSPref{
					{Domain: domain, Key: "someKey", Type: "string"},
				},
			}
			err := validateMacOSPrefs(rc)
			assert.NoError(t, err, "domain %q should be valid", domain)
		})
	}
}

// ---- validatePackageLists ----

func TestValidatePackageLists_ValidNames(t *testing.T) {
	rc := &RemoteConfig{
		Packages: PackageEntryList{{Name: "git"}, {Name: "curl"}, {Name: "node@20"}},
		Casks:    PackageEntryList{{Name: "visual-studio-code"}},
		Taps:     []string{"homebrew/core"},
		Npm:      PackageEntryList{{Name: "@angular/cli"}},
	}
	err := validatePackageLists(rc)
	assert.NoError(t, err)
}

func TestValidatePackageLists_TapMustHaveOneSlash(t *testing.T) {
	// tapNameRe = owner/repo (exactly one slash)
	rc := &RemoteConfig{
		Taps: []string{"homebrew/core"},
	}
	err := validatePackageLists(rc)
	assert.NoError(t, err)
}

func TestValidatePackageLists_TapInvalidFormat(t *testing.T) {
	rc := &RemoteConfig{
		Taps: []string{"notvalid"},
	}
	err := validatePackageLists(rc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid tap name")
}

func TestValidatePackageLists_PackageNameAtMaxLength(t *testing.T) {
	rc := &RemoteConfig{
		Packages: PackageEntryList{{Name: strings.Repeat("a", maxPackageNameLen)}},
	}
	err := validatePackageLists(rc)
	assert.NoError(t, err)
}

func TestValidatePackageLists_PackageNameTooLong(t *testing.T) {
	rc := &RemoteConfig{
		Packages: PackageEntryList{{Name: strings.Repeat("a", maxPackageNameLen+1)}},
	}
	err := validatePackageLists(rc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "package name too long")
}

func TestValidatePackageLists_CaskNameTooLong(t *testing.T) {
	rc := &RemoteConfig{
		Casks: PackageEntryList{{Name: strings.Repeat("b", maxPackageNameLen+1)}},
	}
	err := validatePackageLists(rc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cask name too long")
}

func TestValidatePackageLists_NpmNameTooLong(t *testing.T) {
	rc := &RemoteConfig{
		Npm: PackageEntryList{{Name: strings.Repeat("c", maxPackageNameLen+1)}},
	}
	err := validatePackageLists(rc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "npm package name too long")
}

func TestValidatePackageLists_TapNameTooLong(t *testing.T) {
	// Build a tap name of the form owner/repo that exceeds maxPackageNameLen.
	longTap := strings.Repeat("a", maxPackageNameLen/2) + "/" + strings.Repeat("b", maxPackageNameLen/2)
	rc := &RemoteConfig{
		Taps: []string{longTap},
	}
	err := validatePackageLists(rc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tap name too long")
}

// ---- validatePostInstall ----

func TestValidatePostInstall_ValidCommands(t *testing.T) {
	rc := &RemoteConfig{
		PostInstall: []string{"mise install", "npm install -g pnpm", "echo done"},
	}
	err := validatePostInstall(rc)
	assert.NoError(t, err)
}

func TestValidatePostInstall_EmptySlice(t *testing.T) {
	rc := &RemoteConfig{PostInstall: []string{}}
	err := validatePostInstall(rc)
	assert.NoError(t, err)
}

func TestValidatePostInstall_NilSlice(t *testing.T) {
	rc := &RemoteConfig{}
	err := validatePostInstall(rc)
	assert.NoError(t, err)
}

func TestValidatePostInstall_EmptyCommandRejected(t *testing.T) {
	rc := &RemoteConfig{PostInstall: []string{""}}
	err := validatePostInstall(rc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must not be empty")
}

func TestValidatePostInstall_WhitespaceOnlyRejected(t *testing.T) {
	rc := &RemoteConfig{PostInstall: []string{"   "}}
	err := validatePostInstall(rc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must not be empty")
}

func TestValidatePostInstall_NULByteRejected(t *testing.T) {
	rc := &RemoteConfig{PostInstall: []string{"echo hi\x00"}}
	err := validatePostInstall(rc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "NUL bytes")
}

func TestValidatePostInstall_TooLongRejected(t *testing.T) {
	rc := &RemoteConfig{PostInstall: []string{strings.Repeat("x", maxPostInstallCmdLen+1)}}
	err := validatePostInstall(rc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "too long")
}

func TestValidatePostInstall_AtMaxLengthValid(t *testing.T) {
	rc := &RemoteConfig{PostInstall: []string{strings.Repeat("x", maxPostInstallCmdLen)}}
	err := validatePostInstall(rc)
	assert.NoError(t, err)
}

// ---- Full Validate round-trip ----

func TestRemoteConfigValidate_FullValidConfig(t *testing.T) {
	rc := &RemoteConfig{
		Packages:     PackageEntryList{{Name: "git"}, {Name: "curl"}},
		Casks:        PackageEntryList{{Name: "visual-studio-code"}},
		Taps:         []string{"homebrew/core"},
		Npm:          PackageEntryList{{Name: "typescript"}},
		DotfilesRepo: "https://github.com/alice/dotfiles",
		MacOSPrefs: []RemoteMacOSPref{
			{Domain: "com.apple.dock", Key: "autohide", Type: "bool", Value: "true"},
		},
		PostInstall: []string{"mise install"},
	}
	err := rc.Validate()
	assert.NoError(t, err)
}

func TestRemoteConfigValidate_InvalidDotfilesTriggersError(t *testing.T) {
	rc := &RemoteConfig{
		DotfilesRepo: "git@github.com:user/dotfiles.git",
	}
	err := rc.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid dotfiles_repo")
}
