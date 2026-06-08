package macos

import (
	"fmt"
	"os"

	"github.com/openbootdotdev/openboot/internal/system"
)

// SetDockApps replaces the Dock's pinned-apps list with the given
// absolute paths in order. Missing apps are warned and skipped. After
// success the Dock is restarted via `killall Dock` so the change is
// visible without a logout.
//
// Empty input is treated as "explicitly clear the Dock" — the caller
// (installer) decides nil-vs-empty semantics.
func SetDockApps(apps []string, dryRun bool) error {
	if dryRun {
		fmt.Println("[DRY-RUN] Would clear and rebuild Dock pinned apps:")
		fmt.Println("[DRY-RUN]   defaults delete com.apple.dock persistent-apps")
		for _, app := range apps {
			if _, err := os.Stat(app); err != nil {
				fmt.Printf("[DRY-RUN]   (skip, not installed) %s\n", app)
				continue
			}
			fmt.Printf("[DRY-RUN]   defaults write com.apple.dock persistent-apps -array-add <tile for %s>\n", app)
		}
		fmt.Println("[DRY-RUN]   killall Dock")
		return nil
	}

	// Clear first so the result is fully declarative.
	// Ignore error: key may not exist on a virgin machine.
	_, _ = system.RunCommandSilent("defaults", "delete", "com.apple.dock", "persistent-apps")

	for _, app := range apps {
		if _, err := os.Stat(app); err != nil {
			fmt.Fprintf(os.Stderr, "⚠ Dock: skipping %s (not installed)\n", app)
			continue
		}
		tile := dockTileFor(app)
		if _, err := system.RunCommandSilent("defaults", "write",
			"com.apple.dock", "persistent-apps", "-array-add", tile); err != nil {
			return fmt.Errorf("dock add %s: %w", app, err)
		}
	}

	// killall is best-effort: Dock may not be running on some CI hosts.
	_, _ = system.RunCommandSilent("killall", "Dock")
	return nil
}

// dockTileFor returns the plist-XML <dict> blob that `defaults write
// ... -array-add` expects for one app tile.
func dockTileFor(appPath string) string {
	return fmt.Sprintf(
		`<dict><key>tile-data</key><dict><key>file-data</key><dict>`+
			`<key>_CFURLString</key><string>%s</string>`+
			`<key>_CFURLStringType</key><integer>0</integer>`+
			`</dict></dict></dict>`,
		appPath,
	)
}
