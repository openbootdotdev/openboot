package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/openbootdotdev/openboot/internal/config"
	syncpkg "github.com/openbootdotdev/openboot/internal/sync"
	"github.com/openbootdotdev/openboot/internal/ui"
)

// updateSyncedAt persists the sync source after a successful sync so the
// next `openboot install` (no args) can resume from the same config.
func updateSyncedAt(source *syncpkg.SyncSource, override string, rc *config.RemoteConfig) {
	now := time.Now()
	userSlug := source.UserSlug
	if override != "" {
		userSlug = override
	}
	installedAt := source.InstalledAt
	if installedAt.IsZero() {
		installedAt = now
	}
	updated := &syncpkg.SyncSource{
		UserSlug:    userSlug,
		Username:    rc.Username,
		Slug:        rc.Slug,
		SyncedAt:    now,
		InstalledAt: installedAt,
	}
	if err := syncpkg.SaveSource(updated, false); err != nil {
		ui.Warn(fmt.Sprintf("Failed to update sync source: %v", err))
	}
}

// printInstallDiff renders what will be added/changed on this system if we
// apply the remote config. Extras (local packages not in the config) are
// deliberately not shown — install is additive and does not care about them.
func printInstallDiff(d *syncpkg.SyncDiff) {
	hasPkgAdditions := len(d.MissingFormulae) > 0 || len(d.MissingCasks) > 0 ||
		len(d.MissingNpm) > 0 || len(d.MissingTaps) > 0

	if hasPkgAdditions {
		ui.Printf("  %s\n", ui.Green("Packages to install"))
		printMissing("Formulae", d.MissingFormulae)
		printMissing("Casks", d.MissingCasks)
		printMissing("NPM", d.MissingNpm)
		printMissing("Taps", d.MissingTaps)
		ui.Println()
	}

	if len(d.MacOSChanged) > 0 {
		ui.Printf("  %s\n", ui.Green("macOS Changes"))
		for _, p := range d.MacOSChanged {
			desc := p.Desc
			if desc == "" {
				desc = fmt.Sprintf("%s.%s", p.Domain, p.Key)
			}
			ui.Printf("    %s: %s %s %s\n", desc, p.LocalValue, ui.Yellow("→"), p.RemoteValue)
		}
		ui.Println()
	}

	if d.Shell != nil {
		ui.Printf("  %s\n", ui.Green("Shell Changes"))
		if d.Shell.ThemeChanged {
			localTheme := d.Shell.LocalTheme
			if localTheme == "" {
				localTheme = "(none)"
			}
			ui.Printf("    Theme: %s %s %s\n", localTheme, ui.Yellow("→"), d.Shell.RemoteTheme)
		}
		if d.Shell.PluginsChanged {
			localPlugins := strings.Join(d.Shell.LocalPlugins, ", ")
			if localPlugins == "" {
				localPlugins = "(none)"
			}
			ui.Printf("    Plugins: %s %s %s\n", localPlugins, ui.Yellow("→"), strings.Join(d.Shell.RemotePlugins, ", "))
		}
		ui.Println()
	}

	if d.DotfilesChanged {
		ui.Printf("  %s\n", ui.Green("Dotfiles"))
		ui.Printf("    Repo: %s %s %s\n", fallbackStr(d.LocalDotfiles, "(none)"), ui.Yellow("→"), d.RemoteDotfiles)
		ui.Println()
	}
}

func printMissing(category string, missing []string) {
	if len(missing) == 0 {
		return
	}
	ui.Printf("    %s (%d): %s\n", category, len(missing), strings.Join(missing, ", "))
}

func fallbackStr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// buildInstallPlan converts a diff into a plan that only installs missing items.
// Uninstall fields are never populated — install is additive.
func buildInstallPlan(d *syncpkg.SyncDiff, rc *config.RemoteConfig) *syncpkg.SyncPlan {
	plan := &syncpkg.SyncPlan{
		InstallFormulae: d.MissingFormulae,
		InstallCasks:    d.MissingCasks,
		InstallNpm:      d.MissingNpm,
		InstallTaps:     d.MissingTaps,
	}

	if d.Shell != nil && rc.Shell != nil {
		plan.UpdateShell = true
		plan.ShellOhMyZsh = rc.Shell.OhMyZsh
		plan.ShellTheme = rc.Shell.Theme
		plan.ShellPlugins = rc.Shell.Plugins
	}

	if d.DotfilesChanged {
		plan.UpdateDotfiles = d.RemoteDotfiles
	}

	if len(d.MacOSChanged) > 0 {
		for _, p := range d.MacOSChanged {
			plan.UpdateMacOSPrefs = append(plan.UpdateMacOSPrefs, config.RemoteMacOSPref{
				Domain: p.Domain,
				Key:    p.Key,
				Type:   p.Type,
				Value:  p.RemoteValue,
				Desc:   p.Desc,
				Host:   p.Host,
			})
		}
	}

	return plan
}

// sourceLabel returns a human-readable label for a sync source, preferring
// @username/slug form when available.
func sourceLabel(source *syncpkg.SyncSource) string {
	if source.Username != "" && source.Slug != "" {
		return fmt.Sprintf("@%s/%s", source.Username, source.Slug)
	}
	return source.UserSlug
}

// sourceLabelForConfig builds the label from a RemoteConfig when the sync source
// doesn't have username/slug cached yet (first sync).
func sourceLabelForConfig(rc *config.RemoteConfig) string {
	if rc.Username != "" && rc.Slug != "" {
		return fmt.Sprintf("@%s/%s", rc.Username, rc.Slug)
	}
	return ""
}

// relativeTime returns a human-friendly relative time string.
func relativeTime(d time.Duration) string {
	if d < time.Hour {
		return "just now"
	}
	if d < 24*time.Hour {
		h := int(d.Hours())
		if h == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", h)
	}
	if d < 30*24*time.Hour {
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	}
	if d < 365*24*time.Hour {
		months := int(d.Hours() / 24 / 30)
		if months == 1 {
			return "1 month ago"
		}
		return fmt.Sprintf("%d months ago", months)
	}
	years := int(d.Hours() / 24 / 365)
	if years == 1 {
		return "1 year ago"
	}
	return fmt.Sprintf("%d years ago", years)
}
