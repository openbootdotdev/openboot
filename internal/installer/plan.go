package installer

import (
	"fmt"

	"github.com/openbootdotdev/openboot/internal/config"
	"github.com/openbootdotdev/openboot/internal/dotfiles"
	"github.com/openbootdotdev/openboot/internal/macos"
	"github.com/openbootdotdev/openboot/internal/shell"
	"github.com/openbootdotdev/openboot/internal/system"
	"github.com/openbootdotdev/openboot/internal/ui"
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

	// Packages (fully resolved and categorized)
	Formulae     []string
	Casks        []string
	Npm          []string
	Taps         []string
	SelectedPkgs map[string]bool // for showCompletion and screen-recording reminder
	OnlinePkgs   []config.Package

	// Shell
	InstallOhMyZsh bool
	DotfilesURL    string // "" = skip dotfiles entirely; any URL = use it (may be DefaultDotfilesURL)

	// macOS
	MacOSPrefs []macos.Preference

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
	}

	for _, p := range rc.MacOSPrefs {
		prefType := p.Type
		if prefType == "" {
			prefType = macos.InferPreferenceType(p.Value)
		}
		plan.MacOSPrefs = append(plan.MacOSPrefs, macos.Preference{
			Domain: p.Domain, Key: p.Key, Type: prefType, Value: p.Value, Desc: p.Desc,
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
			return err
		}
		plan.GitName = name
		plan.GitEmail = email
	}

	if err := planPackages(opts, st, plan); err != nil {
		return err
	}

	if !opts.PackagesOnly {
		installShell, err := planShellDecision(opts)
		if err != nil {
			return err
		}
		plan.InstallOhMyZsh = installShell

		dotfilesURL, err := planDotfilesDecision(opts)
		if err != nil {
			return err
		}
		plan.DotfilesURL = dotfilesURL

		macOSPrefs, err := planMacOSDecision(opts)
		if err != nil {
			return err
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
				return err
			}
		}
	}

	if opts.Silent || (opts.DryRun && !system.HasTTY()) {
		st.SelectedPkgs = config.GetPackagesForPreset(opts.Preset)
	} else {
		selected, onlinePkgs, confirmed, err := ui.RunSelector(opts.Preset)
		if err != nil {
			return err
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

	if !opts.Silent && !(opts.DryRun && !system.HasTTY()) {
		setup, err := ui.Confirm("Do you have your own dotfiles repository?", false)
		if err != nil {
			return "", err
		}
		if setup {
			url, err = ui.Input("Dotfiles repository URL (https:// only)", "https://github.com/username/dotfiles")
			if err != nil {
				return "", err
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
	selected, confirmed, err := ui.RunMacOSSelector()
	if err != nil {
		return nil, fmt.Errorf("macOS selector: %w", err)
	}
	if !confirmed {
		return nil, nil
	}
	return selected, nil
}
