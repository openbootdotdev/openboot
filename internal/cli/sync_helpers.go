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
	if err := syncpkg.SaveSource(updated); err != nil {
		ui.Warn(fmt.Sprintf("Failed to update sync source: %v", err))
	}
}

// printSyncDiff renders a human-readable summary of the differences between
// the local system and a remote config. Used by `install` (--dry-run and
// before confirmation) to show what will change.
func printSyncDiff(d *syncpkg.SyncDiff) {
	hasPkgChanges := len(d.MissingFormulae) > 0 || len(d.MissingCasks) > 0 ||
		len(d.MissingNpm) > 0 || len(d.MissingTaps) > 0 ||
		len(d.ExtraFormulae) > 0 || len(d.ExtraCasks) > 0 ||
		len(d.ExtraNpm) > 0 || len(d.ExtraTaps) > 0

	if hasPkgChanges {
		fmt.Printf("  %s\n", ui.Green("Package Changes"))
		printMissingExtra("Formulae", d.MissingFormulae, d.ExtraFormulae)
		printMissingExtra("Casks", d.MissingCasks, d.ExtraCasks)
		printMissingExtra("NPM", d.MissingNpm, d.ExtraNpm)
		printMissingExtra("Taps", d.MissingTaps, d.ExtraTaps)
		fmt.Println()
	}

	if len(d.MacOSChanged) > 0 {
		fmt.Printf("  %s\n", ui.Green("macOS Changes"))
		for _, p := range d.MacOSChanged {
			desc := p.Desc
			if desc == "" {
				desc = fmt.Sprintf("%s.%s", p.Domain, p.Key)
			}
			fmt.Printf("    %s: %s %s %s\n", desc, p.LocalValue, ui.Yellow("→"), p.RemoteValue)
		}
		fmt.Println()
	}

	if d.Shell != nil {
		fmt.Printf("  %s\n", ui.Green("Shell Changes"))
		if d.Shell.ThemeChanged {
			localTheme := d.Shell.LocalTheme
			if localTheme == "" {
				localTheme = "(none)"
			}
			fmt.Printf("    Theme: %s %s %s\n", localTheme, ui.Yellow("→"), d.Shell.RemoteTheme)
		}
		if d.Shell.PluginsChanged {
			localPlugins := strings.Join(d.Shell.LocalPlugins, ", ")
			if localPlugins == "" {
				localPlugins = "(none)"
			}
			fmt.Printf("    Plugins: %s %s %s\n", localPlugins, ui.Yellow("→"), strings.Join(d.Shell.RemotePlugins, ", "))
		}
		fmt.Println()
	}

	if d.DotfilesChanged {
		fmt.Printf("  %s\n", ui.Green("Dotfiles"))
		fmt.Printf("    Repo changed: %s %s %s\n", d.LocalDotfiles, ui.Yellow("→"), d.RemoteDotfiles)
		fmt.Println()
	}
}

func printMissingExtra(category string, missing, extra []string) {
	if len(missing) == 0 && len(extra) == 0 {
		return
	}
	if len(missing) > 0 {
		fmt.Printf("    %s to install (%d): %s\n", category, len(missing), strings.Join(missing, ", "))
	}
	if len(extra) > 0 {
		fmt.Printf("    %s extra (%d): %s\n", category, len(extra), strings.Join(extra, ", "))
	}
}
