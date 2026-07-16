package installer

import (
	"fmt"

	"github.com/openbootdotdev/openboot/internal/config"
	"github.com/openbootdotdev/openboot/internal/dotfiles"
	"github.com/openbootdotdev/openboot/internal/macos"
	"github.com/openbootdotdev/openboot/internal/shell"
	"github.com/openbootdotdev/openboot/internal/system"
	"github.com/openbootdotdev/openboot/internal/ui"
	"github.com/openbootdotdev/openboot/internal/ui/tui"
)

// InstallPlan captures all resolved decisions from the interactive planning phase.
// Nothing has been installed or system state changed when Plan returns; Apply executes it.
type InstallPlan struct {
	Version          string
	DryRun           bool
	Silent           bool
	PackagesOnly     bool
	AllowPostInstall bool

	// Git
	GitName  string
	GitEmail string
	SkipGit  bool // when true, Apply skips git configuration entirely

	// Packages (fully resolved and categorized)
	Formulae     []string
	Casks        []string
	Npm          []string
	Taps         []string
	SelectedPkgs map[string]bool // for showCompletion and screen-recording reminder
	OnlinePkgs   []config.Package

	// Shell
	InstallOhMyZsh bool
	ShellTheme     string   // ZSH_THEME to restore; empty = leave as-is
	ShellPlugins   []string // plugins=(...) to restore; nil = leave as-is
	DotfilesURL    string   // "" = skip dotfiles entirely; any URL = use it (may be DefaultDotfilesURL)

	// macOS
	MacOSPrefs []macos.Preference
	DockApps   []string
	LoginItems []macos.LoginItem

	// Post-install
	PostInstall []string

	// Remote config reference (kept for completion display)
	RemoteConfig *config.RemoteConfig
}

// Plan collects all user decisions and returns a ready-to-Apply InstallPlan.
// All interactive prompts happen here; no packages are installed or files modified.
func Plan(opts *config.InstallOptions, st *config.InstallState) (InstallPlan, error) {
	plan := InstallPlan{
		Version:          opts.Version,
		DryRun:           opts.DryRun,
		Silent:           opts.Silent,
		PackagesOnly:     opts.PackagesOnly,
		AllowPostInstall: opts.AllowPostInstall,
		RemoteConfig:     st.RemoteConfig,
	}

	if st.RemoteConfig != nil {
		planFromRemoteConfig(opts, st, &plan)
		return plan, nil
	}

	return plan, planInteractive(opts, st, &plan)
}

func planFromRemoteConfig(opts *config.InstallOptions, st *config.InstallState, plan *InstallPlan) {
	rc := st.RemoteConfig

	for _, p := range rc.Packages {
		plan.Formulae = append(plan.Formulae, p.Name)
	}
	for _, c := range rc.Casks {
		plan.Casks = append(plan.Casks, c.Name)
	}
	plan.Taps = rc.Taps
	for _, n := range rc.Npm {
		plan.Npm = append(plan.Npm, n.Name)
	}

	switch {
	case rc.DotfilesRepo != "":
		plan.DotfilesURL = rc.DotfilesRepo
	case opts.DotfilesURL != "":
		plan.DotfilesURL = opts.DotfilesURL
	}

	if rc.Shell != nil && rc.Shell.OhMyZsh {
		plan.InstallOhMyZsh = true
		// Carry theme and plugins through so applyShell takes the restore path
		// (writes plugins=() and git-clones external plugins). Dropping these
		// silently downgraded remote-config installs to a bare OMZ install with
		// no plugins cloned.
		plan.ShellTheme = rc.Shell.Theme
		plan.ShellPlugins = rc.Shell.Plugins
	}

	for _, p := range rc.MacOSPrefs {
		prefType := p.Type
		if prefType == "" {
			prefType = macos.InferPreferenceType(p.Value)
		}
		plan.MacOSPrefs = append(plan.MacOSPrefs, macos.Preference{
			Domain: p.Domain, Key: p.Key, Type: prefType, Value: p.Value, Desc: p.Desc, Host: p.Host,
		})
	}

	plan.DockApps = rc.DockApps

	for _, li := range rc.LoginItems {
		plan.LoginItems = append(plan.LoginItems, macos.LoginItem{
			Name:   li.Name,
			Path:   li.Path,
			Hidden: li.Hidden,
		})
	}

	plan.PostInstall = rc.PostInstall

	plan.SelectedPkgs = make(map[string]bool)
	for _, p := range rc.Packages {
		plan.SelectedPkgs[p.Name] = true
	}
	for _, c := range rc.Casks {
		plan.SelectedPkgs[c.Name] = true
	}
	for _, n := range rc.Npm {
		plan.SelectedPkgs[n.Name] = true
	}
}

func planInteractive(opts *config.InstallOptions, st *config.InstallState, plan *InstallPlan) error {
	if !opts.PackagesOnly {
		name, email, err := planGitConfig(opts)
		if err != nil {
			return fmt.Errorf("plan git config: %w", err)
		}
		plan.GitName = name
		plan.GitEmail = email
	}

	if err := planPackages(opts, st, plan); err != nil {
		return fmt.Errorf("plan packages: %w", err)
	}

	if !opts.PackagesOnly {
		installShell, err := planShellDecision(opts)
		if err != nil {
			return fmt.Errorf("plan shell: %w", err)
		}
		plan.InstallOhMyZsh = installShell

		dotfilesURL, err := planDotfilesDecision(opts)
		if err != nil {
			return fmt.Errorf("plan dotfiles: %w", err)
		}
		plan.DotfilesURL = dotfilesURL

		macOSPrefs, err := planMacOSDecision(opts)
		if err != nil {
			return fmt.Errorf("plan macos: %w", err)
		}
		plan.MacOSPrefs = macOSPrefs
	}

	return nil
}

func planGitConfig(opts *config.InstallOptions) (name, email string, err error) {
	existingName, existingEmail := system.GetExistingGitConfig()
	if existingName != "" && existingEmail != "" {
		return existingName, existingEmail, nil
	}

	if opts.DryRun && !system.HasTTY() {
		n := opts.GitName
		if n == "" {
			n = "Your Name"
		}
		e := opts.GitEmail
		if e == "" {
			e = "you@example.com"
		}
		return n, e, nil
	}

	if opts.Silent {
		if opts.GitName == "" || opts.GitEmail == "" {
			return "", "", fmt.Errorf("OPENBOOT_GIT_NAME and OPENBOOT_GIT_EMAIL required in silent mode")
		}
		return opts.GitName, opts.GitEmail, nil
	}

	return ui.InputGitConfig()
}

func planPackages(opts *config.InstallOptions, st *config.InstallState, plan *InstallPlan) error {
	if opts.Preset == "" {
		if opts.Silent || (opts.DryRun && !system.HasTTY()) {
			opts.Preset = "minimal"
		} else {
			var err error
			opts.Preset, err = ui.SelectPreset()
			if err != nil {
				return fmt.Errorf("select preset: %w", err)
			}
		}
	}

	if opts.Silent || (opts.DryRun && !system.HasTTY()) {
		st.SelectedPkgs = config.GetPackagesForPreset(opts.Preset)
	} else {
		selected, onlinePkgs, confirmed, err := tui.RunSelector(opts.Preset)
		if err != nil {
			return fmt.Errorf("run package selector: %w", err)
		}
		if !confirmed {
			return ErrUserCancelled
		}
		st.SelectedPkgs = selected
		st.OnlinePkgs = onlinePkgs
	}

	plan.SelectedPkgs = st.SelectedPkgs
	plan.OnlinePkgs = st.OnlinePkgs

	cats := categorizeSelectedPackages(opts, st)
	plan.Formulae = cats.cli
	plan.Casks = cats.cask
	plan.Npm = cats.npm
	return nil
}

func planShellDecision(opts *config.InstallOptions) (bool, error) {
	if opts.Shell == "skip" {
		return false, nil
	}
	if opts.Shell == "install" {
		return true, nil
	}
	if shell.IsOhMyZshInstalled() {
		return false, nil
	}
	if opts.Silent || (opts.DryRun && !system.HasTTY()) {
		return true, nil
	}
	return ui.Confirm("Install Oh-My-Zsh?", true)
}

func planDotfilesDecision(opts *config.InstallOptions) (string, error) {
	if opts.Dotfiles == "skip" {
		return "", nil
	}

	url := dotfiles.GetDotfilesURL()
	if url == "" {
		url = opts.DotfilesURL
	}
	if url != "" {
		return url, nil
	}

	if !opts.Silent && (!opts.DryRun || system.HasTTY()) {
		setup, err := ui.Confirm("Do you have your own dotfiles repository?", false)
		if err != nil {
			return "", fmt.Errorf("confirm dotfiles: %w", err)
		}
		if setup {
			url, err = ui.Input("Dotfiles repository URL (https:// only)", "https://github.com/username/dotfiles")
			if err != nil {
				return "", fmt.Errorf("input dotfiles url: %w", err)
			}
			if url != "" {
				if vErr := config.ValidateDotfilesURL(url); vErr != nil {
					return "", fmt.Errorf("invalid dotfiles URL: %w", vErr)
				}
				return url, nil
			}
		}
	}

	// No user URL → fall back to default OpenBoot dotfiles
	return dotfiles.DefaultDotfilesURL, nil
}

func planMacOSDecision(opts *config.InstallOptions) ([]macos.Preference, error) {
	if opts.Macos == "skip" {
		return nil, nil
	}
	if opts.Macos == "configure" || opts.Silent || (opts.DryRun && !system.HasTTY()) {
		return macos.DefaultPreferences, nil
	}
	selected, confirmed, err := tui.RunMacOSSelector()
	if err != nil {
		return nil, fmt.Errorf("macOS selector: %w", err)
	}
	if !confirmed {
		return nil, nil
	}
	return selected, nil
}

// PlanFromSelection builds a ready-to-Apply InstallPlan from an explicit
// package selection gathered by the install TUI. online carries packages the
// wizard picked from openboot.dev search — they aren't in the local catalog,
// so categorization needs their type info. It applies system-config
// defaults (existing git identity, oh-my-zsh, dotfiles, macOS prefs) without
// any interactive prompts — all interaction already happened in the wizard.
//
// Git identity is reused from the existing global git config when present; when
// absent the git step is skipped rather than prompting, since the TUI has no
// name/email screen. CLI overrides (--packages-only, --shell/--macos/--dotfiles
// skip) are still honored via opts.
func PlanFromSelection(opts *config.InstallOptions, selected map[string]bool, online []config.Package) InstallPlan {
	st := &config.InstallState{SelectedPkgs: selected, OnlinePkgs: online}

	plan := InstallPlan{
		Version:          opts.Version,
		DryRun:           opts.DryRun,
		Silent:           opts.Silent,
		PackagesOnly:     opts.PackagesOnly,
		AllowPostInstall: opts.AllowPostInstall,
		SelectedPkgs:     selected,
		OnlinePkgs:       online,
	}

	cats := categorizeSelectedPackages(opts, st)
	plan.Formulae = cats.cli
	plan.Casks = cats.cask
	plan.Npm = cats.npm

	if opts.PackagesOnly {
		return plan
	}

	name, email := system.GetExistingGitConfig()
	if name != "" && email != "" {
		plan.GitName = name
		plan.GitEmail = email
	} else {
		plan.SkipGit = true
	}

	plan.InstallOhMyZsh = opts.Shell != "skip"

	if opts.Dotfiles != "skip" {
		url := dotfiles.GetDotfilesURL()
		if url == "" {
			url = opts.DotfilesURL
		}
		if url == "" {
			url = dotfiles.DefaultDotfilesURL
		}
		plan.DotfilesURL = url
	}

	if opts.Macos != "skip" {
		plan.MacOSPrefs = macos.DefaultPreferences
	}

	return plan
}

// PlanForRemoteSelection builds a ready-to-Apply InstallPlan from a remote
// config filtered to the wizard's package selection, plus any openboot.dev
// search picks made on the select screen. It is the config-mode counterpart
// of PlanFromSelection: everything non-package (git, dotfiles, shell, macOS
// prefs, dock, login items, post-install) comes from the remote config via
// planFromRemoteConfig, exactly like the declarative slug path — no prompts.
func PlanForRemoteSelection(opts *config.InstallOptions, rc *config.RemoteConfig, selected map[string]bool, online []config.Package) InstallPlan {
	f := *rc
	f.Packages = filterEntriesBySelection(rc.Packages, selected)
	f.Casks = filterEntriesBySelection(rc.Casks, selected)
	f.Npm = filterEntriesBySelection(rc.Npm, selected)

	plan := InstallPlan{
		Version:          opts.Version,
		DryRun:           opts.DryRun,
		Silent:           opts.Silent,
		PackagesOnly:     opts.PackagesOnly,
		AllowPostInstall: opts.AllowPostInstall,
		RemoteConfig:     &f,
	}
	st := &config.InstallState{RemoteConfig: &f}
	planFromRemoteConfig(opts, st, &plan)

	for _, p := range online {
		if !selected[p.Name] || plan.SelectedPkgs[p.Name] {
			continue
		}
		switch {
		case p.IsNpm:
			plan.Npm = append(plan.Npm, p.Name)
		case p.IsCask:
			plan.Casks = append(plan.Casks, p.Name)
		default:
			plan.Formulae = append(plan.Formulae, p.Name)
		}
		plan.SelectedPkgs[p.Name] = true
		plan.OnlinePkgs = append(plan.OnlinePkgs, p)
	}
	return plan
}

func filterEntriesBySelection(in config.PackageEntryList, selected map[string]bool) config.PackageEntryList {
	out := make(config.PackageEntryList, 0, len(in))
	for _, e := range in {
		if selected[e.Name] {
			out = append(out, e)
		}
	}
	return out
}

// PlanFromSnapshot builds an InstallPlan from snapshot state without any interactive
// prompts. All decisions are derived from st.Snapshot* fields and opts.
func PlanFromSnapshot(opts *config.InstallOptions, st *config.InstallState) InstallPlan {
	plan := InstallPlan{
		Version:          opts.Version,
		DryRun:           opts.DryRun,
		Silent:           opts.Silent,
		PackagesOnly:     opts.PackagesOnly,
		AllowPostInstall: opts.AllowPostInstall,
		Taps:             st.SnapshotTaps,
		SelectedPkgs:     st.SelectedPkgs,
	}

	// Categorize selected packages into formulae, casks, and npm.
	cats := categorizeSelectedPackages(opts, st)
	plan.Formulae = cats.cli
	plan.Casks = cats.cask
	plan.Npm = cats.npm

	// Git: restore from snapshot when present; skip entirely when snapshot has no git config.
	if st.SnapshotGit != nil {
		plan.GitName = st.SnapshotGit.UserName
		plan.GitEmail = st.SnapshotGit.UserEmail
	} else {
		plan.SkipGit = true
	}

	// Dotfiles: non-empty snapshot URL means apply, unless explicitly skipped via flag.
	if opts.Dotfiles != "skip" {
		plan.DotfilesURL = st.SnapshotDotfiles
	}

	// Shell: restore exactly what the snapshot recorded; don't install OMZ if it wasn't there.
	if opts.Shell != "skip" && st.SnapshotShellOhMyZsh {
		plan.InstallOhMyZsh = true
		plan.ShellTheme = st.SnapshotShellTheme
		plan.ShellPlugins = st.SnapshotShellPlugins
	}

	// macOS: convert snapshot preferences to macos.Preference values, unless skipped via flag.
	if opts.Macos != "skip" && len(st.SnapshotMacOS) > 0 {
		prefs := make([]macos.Preference, 0, len(st.SnapshotMacOS))
		for _, p := range st.SnapshotMacOS {
			prefType := p.Type
			if prefType == "" {
				prefType = macos.InferPreferenceType(p.Value)
			}
			prefs = append(prefs, macos.Preference{
				Domain: p.Domain,
				Key:    p.Key,
				Type:   prefType,
				Value:  p.Value,
				Desc:   p.Desc,
				Host:   p.Host,
			})
		}
		plan.MacOSPrefs = prefs
	}

	return plan
}
