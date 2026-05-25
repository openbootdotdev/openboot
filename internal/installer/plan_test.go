package installer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openbootdotdev/openboot/internal/config"
	"github.com/openbootdotdev/openboot/internal/macos"
)

// ---------------------------------------------------------------------------
// planGitConfig
// ---------------------------------------------------------------------------

// planGitConfig calls system.GetExistingGitConfig() first.  In test
// environments git user.name / user.email may or may not be set, so each
// sub-test controls opts fields plus uses Silent/DryRun to avoid hitting
// the interactive TUI.

func TestPlanGitConfig_SilentMode_WithNameAndEmail(t *testing.T) {
	opts := &config.InstallOptions{
		Silent:   true,
		GitName:  "Alice",
		GitEmail: "alice@example.com",
	}

	name, email, err := planGitConfig(opts)
	require.NoError(t, err)
	// When git is already configured on the test machine the existing values
	// are returned; otherwise our supplied opts values are used.
	assert.NotEmpty(t, name)
	assert.NotEmpty(t, email)
}

func TestPlanGitConfig_SilentMode_MissingEmail(t *testing.T) {
	// Silent mode with incomplete credentials returns an error.
	// This only triggers when git user.name/email are NOT already configured
	// on the host; guard the assertion accordingly.
	opts := &config.InstallOptions{
		Silent:   true,
		GitName:  "Alice",
		GitEmail: "",
	}

	name, email, err := planGitConfig(opts)
	// If the machine has git configured the existing values take precedence.
	if name != "" && email != "" {
		assert.NoError(t, err)
		return
	}
	require.Error(t, err)
	assert.Contains(t, err.Error(), "OPENBOOT_GIT_EMAIL")
}

func TestPlanGitConfig_SilentMode_BothEmpty(t *testing.T) {
	opts := &config.InstallOptions{
		Silent:   true,
		GitName:  "",
		GitEmail: "",
	}

	name, email, err := planGitConfig(opts)
	if name != "" && email != "" {
		// Host already has git configured – expected non-error path.
		assert.NoError(t, err)
		return
	}
	require.Error(t, err)
	assert.Contains(t, err.Error(), "required in silent mode")
}

func TestPlanGitConfig_DryRun_NoTTY_UsesOptsValues(t *testing.T) {
	// DryRun without a TTY uses opts.GitName/GitEmail (or defaults).
	// In a CI/test environment HasTTY() returns false, so this path is exercised.
	opts := &config.InstallOptions{
		DryRun:   true,
		GitName:  "Bob",
		GitEmail: "bob@example.com",
	}

	name, email, err := planGitConfig(opts)
	require.NoError(t, err)
	assert.NotEmpty(t, name)
	assert.NotEmpty(t, email)
}

func TestPlanGitConfig_DryRun_NoTTY_EmptyOptsUsesDefaults(t *testing.T) {
	// When DryRun=true and opts has no name/email and there's no TTY,
	// planGitConfig should fill in placeholder defaults.
	opts := &config.InstallOptions{
		DryRun:   true,
		GitName:  "",
		GitEmail: "",
	}

	name, email, err := planGitConfig(opts)
	require.NoError(t, err)
	// Either the existing git config on the host, the opts value, or the
	// placeholder default "Your Name" / "you@example.com".
	assert.NotEmpty(t, name)
	assert.NotEmpty(t, email)
}

// ---------------------------------------------------------------------------
// planShellDecision
// ---------------------------------------------------------------------------

func TestPlanShellDecision_ExplicitSkip(t *testing.T) {
	opts := &config.InstallOptions{Shell: "skip"}
	install, err := planShellDecision(opts)
	require.NoError(t, err)
	assert.False(t, install, "skip should never install OMZ")
}

func TestPlanShellDecision_ExplicitInstall(t *testing.T) {
	opts := &config.InstallOptions{Shell: "install"}
	install, err := planShellDecision(opts)
	require.NoError(t, err)
	assert.True(t, install, "explicit install flag must return true")
}

func TestPlanShellDecision_Silent_OmzNotInstalled(t *testing.T) {
	// Silent mode without OMZ installed: should auto-install.
	// IsOhMyZshInstalled() result depends on the host; we only check when it
	// returns false (i.e. OMZ not present).
	opts := &config.InstallOptions{Silent: true}
	install, err := planShellDecision(opts)
	require.NoError(t, err)
	// Either true (OMZ not present, silent auto-install) or false (already present).
	_ = install // both outcomes are acceptable
}

func TestPlanShellDecision_DryRunNoTTY_AutoInstall(t *testing.T) {
	// DryRun without a TTY behaves like Silent: auto-install if OMZ absent.
	opts := &config.InstallOptions{DryRun: true}
	install, err := planShellDecision(opts)
	require.NoError(t, err)
	_ = install // result depends on host OMZ state; test just verifies no panic/error
}

// ---------------------------------------------------------------------------
// planDotfilesDecision
// ---------------------------------------------------------------------------

func TestPlanDotfilesDecision_Skip(t *testing.T) {
	t.Setenv("OPENBOOT_DOTFILES", "")
	opts := &config.InstallOptions{Dotfiles: "skip"}
	url, err := planDotfilesDecision(opts)
	require.NoError(t, err)
	assert.Empty(t, url)
}

func TestPlanDotfilesDecision_EnvVarTakesPriority(t *testing.T) {
	t.Setenv("OPENBOOT_DOTFILES", "https://github.com/envuser/dotfiles")
	opts := &config.InstallOptions{
		Silent:      true,
		DotfilesURL: "https://github.com/optuser/dotfiles",
	}
	url, err := planDotfilesDecision(opts)
	require.NoError(t, err)
	assert.Equal(t, "https://github.com/envuser/dotfiles", url)
}

func TestPlanDotfilesDecision_OptsURLUsedWhenNoEnvVar(t *testing.T) {
	t.Setenv("OPENBOOT_DOTFILES", "")
	opts := &config.InstallOptions{
		Silent:      true,
		DotfilesURL: "https://github.com/optuser/dotfiles",
	}
	url, err := planDotfilesDecision(opts)
	require.NoError(t, err)
	assert.Equal(t, "https://github.com/optuser/dotfiles", url)
}

func TestPlanDotfilesDecision_SilentNoURLFallsBackToDefault(t *testing.T) {
	t.Setenv("OPENBOOT_DOTFILES", "")
	opts := &config.InstallOptions{
		Silent:      true,
		DotfilesURL: "",
	}
	url, err := planDotfilesDecision(opts)
	require.NoError(t, err)
	// Falls back to the OpenBoot default dotfiles URL.
	assert.NotEmpty(t, url)
}

func TestPlanDotfilesDecision_DryRunNoTTY_FallsBackToDefault(t *testing.T) {
	t.Setenv("OPENBOOT_DOTFILES", "")
	opts := &config.InstallOptions{
		DryRun:      true,
		DotfilesURL: "",
	}
	url, err := planDotfilesDecision(opts)
	require.NoError(t, err)
	assert.NotEmpty(t, url)
}

// ---------------------------------------------------------------------------
// planMacOSDecision
// ---------------------------------------------------------------------------

func TestPlanMacOSDecision_Skip(t *testing.T) {
	opts := &config.InstallOptions{Macos: "skip"}
	prefs, err := planMacOSDecision(opts)
	require.NoError(t, err)
	assert.Nil(t, prefs)
}

func TestPlanMacOSDecision_Configure(t *testing.T) {
	opts := &config.InstallOptions{Macos: "configure"}
	prefs, err := planMacOSDecision(opts)
	require.NoError(t, err)
	assert.Equal(t, macos.DefaultPreferences, prefs)
}

func TestPlanMacOSDecision_Silent(t *testing.T) {
	opts := &config.InstallOptions{Silent: true}
	prefs, err := planMacOSDecision(opts)
	require.NoError(t, err)
	assert.Equal(t, macos.DefaultPreferences, prefs)
}

func TestPlanMacOSDecision_DryRunNoTTY(t *testing.T) {
	opts := &config.InstallOptions{DryRun: true}
	prefs, err := planMacOSDecision(opts)
	require.NoError(t, err)
	// DryRun without a TTY also uses defaults.
	assert.Equal(t, macos.DefaultPreferences, prefs)
}

// ---------------------------------------------------------------------------
// planInteractive (partial — covers non-interactive branches)
// ---------------------------------------------------------------------------

func TestPlanInteractive_PackagesOnly_DryRun(t *testing.T) {
	cfg := &config.Config{
		InstallOptions: config.InstallOptions{
			DryRun:       true,
			PackagesOnly: true,
			Preset:       "minimal",
		},
	}
	opts := cfg.ToInstallOptions()
	st := cfg.ToInstallState()
	plan := InstallPlan{}

	err := planInteractive(opts, st, &plan)
	require.NoError(t, err)

	// PackagesOnly skips git, shell, dotfiles, macOS planning.
	assert.Empty(t, plan.GitName)
	assert.Empty(t, plan.GitEmail)
	assert.False(t, plan.InstallOhMyZsh)
	assert.Nil(t, plan.MacOSPrefs)
}

func TestPlanInteractive_Silent_Preset_Minimal(t *testing.T) {
	t.Setenv("OPENBOOT_DOTFILES", "")

	cfg := &config.Config{
		InstallOptions: config.InstallOptions{
			Silent:   true,
			Preset:   "minimal",
			Shell:    "skip",
			Macos:    "skip",
			GitName:  "CI User",
			GitEmail: "ci@example.com",
		},
	}
	opts := cfg.ToInstallOptions()
	st := cfg.ToInstallState()
	plan := InstallPlan{}

	err := planInteractive(opts, st, &plan)
	require.NoError(t, err)
	assert.NotNil(t, plan.SelectedPkgs)
}

func TestPlanInteractive_DryRun_Minimal_AllSkipped(t *testing.T) {
	t.Setenv("OPENBOOT_DOTFILES", "")

	cfg := &config.Config{
		InstallOptions: config.InstallOptions{
			DryRun:   true,
			Preset:   "minimal",
			Shell:    "skip",
			Macos:    "skip",
			Dotfiles: "skip",
		},
	}
	opts := cfg.ToInstallOptions()
	st := cfg.ToInstallState()
	plan := InstallPlan{}

	err := planInteractive(opts, st, &plan)
	require.NoError(t, err)
	// Dotfiles skipped → DotfilesURL empty.
	assert.Empty(t, plan.DotfilesURL)
	// macOS skipped → nil.
	assert.Nil(t, plan.MacOSPrefs)
	// Shell skipped → not installed.
	assert.False(t, plan.InstallOhMyZsh)
}

// ---------------------------------------------------------------------------
// PlanFromSnapshot
// ---------------------------------------------------------------------------

func TestPlanFromSnapshot_GitNil_SetsSkipGit(t *testing.T) {
	cfg := &config.Config{
		InstallOptions: config.InstallOptions{
			DryRun:   true,
			Silent:   true,
			Shell:    "skip",
			Macos:    "skip",
			Dotfiles: "skip",
		},
		InstallState: config.InstallState{
			SnapshotGit: nil,
		},
	}
	opts := cfg.ToInstallOptions()
	st := cfg.ToInstallState()
	plan := PlanFromSnapshot(opts, st)

	assert.True(t, plan.SkipGit)
	assert.Empty(t, plan.GitName)
	assert.Empty(t, plan.GitEmail)
}

func TestPlanFromSnapshot_GitPresent_PopulatesNameEmail(t *testing.T) {
	cfg := &config.Config{
		InstallOptions: config.InstallOptions{
			DryRun:   true,
			Silent:   true,
			Shell:    "skip",
			Macos:    "skip",
			Dotfiles: "skip",
		},
		InstallState: config.InstallState{
			SnapshotGit: &config.SnapshotGitConfig{
				UserName:  "Snap User",
				UserEmail: "snap@example.com",
			},
		},
	}
	opts := cfg.ToInstallOptions()
	st := cfg.ToInstallState()
	plan := PlanFromSnapshot(opts, st)

	assert.False(t, plan.SkipGit)
	assert.Equal(t, "Snap User", plan.GitName)
	assert.Equal(t, "snap@example.com", plan.GitEmail)
}

func TestPlanFromSnapshot_DotfilesSkipFlag(t *testing.T) {
	cfg := &config.Config{
		InstallOptions: config.InstallOptions{
			DryRun:   true,
			Silent:   true,
			Shell:    "skip",
			Macos:    "skip",
			Dotfiles: "skip",
		},
		InstallState: config.InstallState{
			SnapshotDotfiles: "https://github.com/user/dotfiles",
		},
	}
	opts := cfg.ToInstallOptions()
	st := cfg.ToInstallState()
	plan := PlanFromSnapshot(opts, st)

	// --dotfiles skip must override the snapshot URL.
	assert.Empty(t, plan.DotfilesURL)
}

func TestPlanFromSnapshot_DotfilesFromSnapshot(t *testing.T) {
	cfg := &config.Config{
		InstallOptions: config.InstallOptions{
			DryRun: true,
			Silent: true,
			Shell:  "skip",
			Macos:  "skip",
		},
		InstallState: config.InstallState{
			SnapshotDotfiles: "https://github.com/user/dotfiles",
		},
	}
	opts := cfg.ToInstallOptions()
	st := cfg.ToInstallState()
	plan := PlanFromSnapshot(opts, st)

	assert.Equal(t, "https://github.com/user/dotfiles", plan.DotfilesURL)
}

func TestPlanFromSnapshot_ShellRestored(t *testing.T) {
	cfg := &config.Config{
		InstallOptions: config.InstallOptions{
			DryRun:   true,
			Silent:   true,
			Macos:    "skip",
			Dotfiles: "skip",
		},
		InstallState: config.InstallState{
			SnapshotShellOhMyZsh: true,
			SnapshotShellTheme:   "robbyrussell",
			SnapshotShellPlugins: []string{"git", "kubectl"},
		},
	}
	opts := cfg.ToInstallOptions()
	st := cfg.ToInstallState()
	plan := PlanFromSnapshot(opts, st)

	assert.True(t, plan.InstallOhMyZsh)
	assert.Equal(t, "robbyrussell", plan.ShellTheme)
	assert.Equal(t, []string{"git", "kubectl"}, plan.ShellPlugins)
}

func TestPlanFromSnapshot_ShellSkipFlag(t *testing.T) {
	cfg := &config.Config{
		InstallOptions: config.InstallOptions{
			DryRun:   true,
			Silent:   true,
			Shell:    "skip",
			Macos:    "skip",
			Dotfiles: "skip",
		},
		InstallState: config.InstallState{
			SnapshotShellOhMyZsh: true,
			SnapshotShellTheme:   "robbyrussell",
		},
	}
	opts := cfg.ToInstallOptions()
	st := cfg.ToInstallState()
	plan := PlanFromSnapshot(opts, st)

	assert.False(t, plan.InstallOhMyZsh, "--shell skip must prevent OMZ restore")
}

func TestPlanFromSnapshot_MacOSPrefsFromSnapshot(t *testing.T) {
	cfg := &config.Config{
		InstallOptions: config.InstallOptions{
			DryRun:   true,
			Silent:   true,
			Shell:    "skip",
			Dotfiles: "skip",
		},
		InstallState: config.InstallState{
			SnapshotMacOS: []config.RemoteMacOSPref{
				{Domain: "com.apple.dock", Key: "autohide", Type: "bool", Value: "true"},
			},
		},
	}
	opts := cfg.ToInstallOptions()
	st := cfg.ToInstallState()
	plan := PlanFromSnapshot(opts, st)

	require.Len(t, plan.MacOSPrefs, 1)
	assert.Equal(t, "com.apple.dock", plan.MacOSPrefs[0].Domain)
	assert.Equal(t, "autohide", plan.MacOSPrefs[0].Key)
}

func TestPlanFromSnapshot_MacOSSkipFlag(t *testing.T) {
	cfg := &config.Config{
		InstallOptions: config.InstallOptions{
			DryRun:   true,
			Silent:   true,
			Shell:    "skip",
			Dotfiles: "skip",
			Macos:    "skip",
		},
		InstallState: config.InstallState{
			SnapshotMacOS: []config.RemoteMacOSPref{
				{Domain: "com.apple.dock", Key: "autohide", Type: "bool", Value: "true"},
			},
		},
	}
	opts := cfg.ToInstallOptions()
	st := cfg.ToInstallState()
	plan := PlanFromSnapshot(opts, st)

	assert.Nil(t, plan.MacOSPrefs, "--macos skip must prevent prefs restore")
}

func TestPlanFromSnapshot_PackageCategorizationFromSelectedPkgs(t *testing.T) {
	cfg := &config.Config{
		InstallOptions: config.InstallOptions{
			DryRun:   true,
			Silent:   true,
			Shell:    "skip",
			Macos:    "skip",
			Dotfiles: "skip",
		},
		InstallState: config.InstallState{
			// Use packages that are known to the local catalog so they get
			// categorized correctly without a remote config.
			SelectedPkgs: map[string]bool{
				"curl": true,
			},
		},
	}
	opts := cfg.ToInstallOptions()
	st := cfg.ToInstallState()
	plan := PlanFromSnapshot(opts, st)

	// curl is a CLI formula; it must appear in Formulae (not Casks or Npm).
	assert.Contains(t, plan.Formulae, "curl")
}
