package cli

import (
	"testing"
	"time"

	"github.com/openbootdotdev/openboot/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyEnvOverrides_SilentMode(t *testing.T) {
	cfg := &config.Config{Silent: true}

	t.Setenv("OPENBOOT_GIT_NAME", "Test User")
	t.Setenv("OPENBOOT_GIT_EMAIL", "test@example.com")
	t.Setenv("OPENBOOT_PRESET", "developer")

	applyEnvOverrides(cfg)

	assert.Equal(t, "Test User", cfg.GitName)
	assert.Equal(t, "test@example.com", cfg.GitEmail)
	assert.Equal(t, "developer", cfg.Preset)
}

func TestApplyEnvOverrides_GitEnvIgnoredWhenNotSilent(t *testing.T) {
	cfg := &config.Config{Silent: false}

	t.Setenv("OPENBOOT_GIT_NAME", "Test User")
	t.Setenv("OPENBOOT_GIT_EMAIL", "test@example.com")

	applyEnvOverrides(cfg)

	assert.Empty(t, cfg.GitName)
	assert.Empty(t, cfg.GitEmail)
}

func TestApplyEnvOverrides_UserFromEnv(t *testing.T) {
	cfg := &config.Config{}

	t.Setenv("OPENBOOT_USER", "alice")

	applyEnvOverrides(cfg)

	assert.Equal(t, "alice", cfg.User)
}

func TestApplyEnvOverrides_FlagTakesPrecedenceOverEnv(t *testing.T) {
	cfg := &config.Config{Preset: "minimal", User: "bob"}

	t.Setenv("OPENBOOT_PRESET", "full")
	t.Setenv("OPENBOOT_USER", "alice")

	applyEnvOverrides(cfg)

	// Flags set before env override; env must not overwrite explicit flag values.
	assert.Equal(t, "minimal", cfg.Preset)
	assert.Equal(t, "bob", cfg.User)
}

func TestLooksLikeFilePath(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"relative dot", "./file.json", true},
		{"relative dotdot", "../file.json", true},
		{"absolute", "/tmp/file.json", true},
		{"json suffix", "backup.json", true},
		{"user slug", "alice/dev-setup", false},
		{"plain word", "developer", false},
		{"empty", "", false},
		{"no slash json", "backup.txt", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, looksLikeFilePath(tt.in))
		})
	}
}

func TestLooksLikeUserSlug(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"standard", "alice/dev-setup", true},
		{"underscores", "alice_b/my_setup", true},
		{"digits", "user123/config-2", true},
		{"leading digits in slug", "alice/2-config", true},
		{"leading digit user", "1alice/foo", true},
		{"leading dash", "-alice/foo", false},
		{"three parts", "a/b/c", false},
		{"trailing slash", "alice/", false},
		{"just slash", "/", false},
		{"no slash", "alice", false},
		{"file-like", "./alice/foo", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, looksLikeUserSlug(tt.in))
		})
	}
}

func TestResolvePositionalArg_Preset(t *testing.T) {
	src, err := resolvePositionalArg("developer")
	assert.NoError(t, err)
	// "developer" is a built-in preset name.
	assert.Equal(t, sourcePreset, src.kind)
}

func TestResolvePositionalArg_File(t *testing.T) {
	src, err := resolvePositionalArg("./backup.json")
	assert.NoError(t, err)
	assert.Equal(t, sourceFile, src.kind)
	assert.Equal(t, "./backup.json", src.path)
}

func TestResolvePositionalArg_UserSlug(t *testing.T) {
	src, err := resolvePositionalArg("alice/dev-setup")
	assert.NoError(t, err)
	assert.Equal(t, sourceCloud, src.kind)
	assert.Equal(t, "alice/dev-setup", src.userSlug)
}

func TestResolvePositionalArg_Alias(t *testing.T) {
	// A plain word that isn't a preset is treated as a cloud alias —
	// FetchRemoteConfig will attempt alias resolution at run time.
	src, err := resolvePositionalArg("my-custom-alias")
	assert.NoError(t, err)
	assert.Equal(t, sourceCloud, src.kind)
	assert.Equal(t, "my-custom-alias", src.userSlug)
}

func TestRelativeTime(t *testing.T) {
	tests := []struct {
		name     string
		d        time.Duration
		contains string
	}{
		{"under an hour", 30 * time.Minute, "just now"},
		{"hours", 2 * time.Hour, "hours"},
		{"one hour singular", 1 * time.Hour, "1 hour ago"},
		{"days", 3 * 24 * time.Hour, "days"},
		{"one day singular", 24 * time.Hour, "1 day ago"},
		{"months", 45 * 24 * time.Hour, "month"},
		{"years", 400 * 24 * time.Hour, "year"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Contains(t, relativeTime(tt.d), tt.contains)
		})
	}
}

func TestResolveInstallSource_FromFlag(t *testing.T) {
	cmd := installCmd
	// reset the --from flag value between subtests
	require.NoError(t, cmd.Flags().Set("from", ""))

	t.Run("from flag takes precedence over args", func(t *testing.T) {
		require.NoError(t, cmd.Flags().Set("from", "/tmp/config.json"))
		t.Cleanup(func() { _ = cmd.Flags().Set("from", "") })

		src, err := resolveInstallSource(cmd, []string{"developer"})
		require.NoError(t, err)
		assert.Equal(t, sourceFile, src.kind)
		assert.Equal(t, "/tmp/config.json", src.path)
	})
}

func TestResolveInstallSource_UserFlag(t *testing.T) {
	// Save and restore installCfg.User
	orig := installCfg.User
	t.Cleanup(func() { installCfg.User = orig })

	installCfg.User = "alice/setup"
	cmd := installCmd
	require.NoError(t, cmd.Flags().Set("from", ""))

	src, err := resolveInstallSource(cmd, []string{})
	require.NoError(t, err)
	assert.Equal(t, sourceCloud, src.kind)
	assert.Equal(t, "alice/setup", src.userSlug)
}

func TestResolveInstallSource_PresetFlag(t *testing.T) {
	// Save and restore installCfg.Preset and installCfg.User
	origPreset := installCfg.Preset
	origUser := installCfg.User
	t.Cleanup(func() {
		installCfg.Preset = origPreset
		installCfg.User = origUser
	})

	installCfg.Preset = "minimal"
	installCfg.User = ""
	cmd := installCmd
	require.NoError(t, cmd.Flags().Set("from", ""))

	src, err := resolveInstallSource(cmd, []string{})
	require.NoError(t, err)
	assert.Equal(t, sourcePreset, src.kind)
}

func TestResolveInstallSource_NoArgs_NoSyncSource(t *testing.T) {
	// No flags, no args, no sync source on disk → sourceNone
	origPreset := installCfg.Preset
	origUser := installCfg.User
	t.Cleanup(func() {
		installCfg.Preset = origPreset
		installCfg.User = origUser
	})

	installCfg.Preset = ""
	installCfg.User = ""

	// Use a temp HOME with no .openboot directory so LoadSource returns nil.
	t.Setenv("HOME", t.TempDir())

	cmd := installCmd
	require.NoError(t, cmd.Flags().Set("from", ""))

	src, err := resolveInstallSource(cmd, []string{})
	require.NoError(t, err)
	assert.Equal(t, sourceNone, src.kind)
}

func TestResolveInstallSource_PositionalFile(t *testing.T) {
	origPreset := installCfg.Preset
	origUser := installCfg.User
	t.Cleanup(func() {
		installCfg.Preset = origPreset
		installCfg.User = origUser
	})

	installCfg.Preset = ""
	installCfg.User = ""

	t.Setenv("HOME", t.TempDir())
	cmd := installCmd
	require.NoError(t, cmd.Flags().Set("from", ""))

	src, err := resolveInstallSource(cmd, []string{"./my-config.json"})
	require.NoError(t, err)
	assert.Equal(t, sourceFile, src.kind)
	assert.Equal(t, "./my-config.json", src.path)
}

func TestResolveInstallSource_PositionalUserSlug(t *testing.T) {
	origPreset := installCfg.Preset
	origUser := installCfg.User
	t.Cleanup(func() {
		installCfg.Preset = origPreset
		installCfg.User = origUser
	})

	installCfg.Preset = ""
	installCfg.User = ""

	t.Setenv("HOME", t.TempDir())
	cmd := installCmd
	require.NoError(t, cmd.Flags().Set("from", ""))

	src, err := resolveInstallSource(cmd, []string{"bob/dev-env"})
	require.NoError(t, err)
	assert.Equal(t, sourceCloud, src.kind)
	assert.Equal(t, "bob/dev-env", src.userSlug)
}

func TestResolveInstallSource_PositionalPreset(t *testing.T) {
	origPreset := installCfg.Preset
	origUser := installCfg.User
	t.Cleanup(func() {
		installCfg.Preset = origPreset
		installCfg.User = origUser
	})

	installCfg.Preset = ""
	installCfg.User = ""

	t.Setenv("HOME", t.TempDir())
	cmd := installCmd
	require.NoError(t, cmd.Flags().Set("from", ""))

	src, err := resolveInstallSource(cmd, []string{"full"})
	require.NoError(t, err)
	assert.Equal(t, sourcePreset, src.kind)
}
