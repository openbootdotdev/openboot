package macos

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Preference struct {
	Domain string
	Key    string
	Type   string
	Value  string
	Desc   string
}

var DefaultPreferences = []Preference{
	{"NSGlobalDomain", "AppleShowAllExtensions", "bool", "true", "Show all file extensions"},
	{"NSGlobalDomain", "AppleShowScrollBars", "string", "Always", "Always show scrollbars"},
	{"NSGlobalDomain", "NSAutomaticSpellingCorrectionEnabled", "bool", "false", "Disable auto-correct"},
	{"NSGlobalDomain", "NSAutomaticCapitalizationEnabled", "bool", "false", "Disable auto-capitalization"},
	{"NSGlobalDomain", "KeyRepeat", "int", "2", "Fast key repeat rate"},
	{"NSGlobalDomain", "InitialKeyRepeat", "int", "15", "Short delay until key repeat"},

	{"com.apple.finder", "ShowPathbar", "bool", "true", "Show path bar in Finder"},
	{"com.apple.finder", "ShowStatusBar", "bool", "true", "Show status bar in Finder"},
	{"com.apple.finder", "FXPreferredViewStyle", "string", "Nlsv", "Use list view in Finder"},
	{"com.apple.finder", "FXEnableExtensionChangeWarning", "bool", "false", "No extension change warning"},
	{"com.apple.finder", "AppleShowAllFiles", "bool", "true", "Show hidden files in Finder"},

	{"com.apple.dock", "autohide", "bool", "false", "Keep Dock visible"},
	{"com.apple.dock", "show-recents", "bool", "false", "Don't show recent apps in Dock"},
	{"com.apple.dock", "tilesize", "int", "48", "Set Dock icon size"},
	{"com.apple.dock", "mineffect", "string", "scale", "Minimize windows with scale effect"},

	{"com.apple.screencapture", "location", "string", "~/Screenshots", "Save screenshots to ~/Screenshots"},
	{"com.apple.screencapture", "type", "string", "png", "Save screenshots as PNG"},
	{"com.apple.screencapture", "disable-shadow", "bool", "true", "Disable screenshot shadows"},

	{"com.apple.Safari", "IncludeDevelopMenu", "bool", "true", "Enable Safari Developer menu"},
	{"com.apple.Safari", "WebKitDeveloperExtrasEnabledPreferenceKey", "bool", "true", "Enable Safari WebKit dev extras"},

	{"com.apple.TextEdit", "RichText", "bool", "false", "Use plain text in TextEdit"},
	{"com.apple.TextEdit", "PlainTextEncoding", "int", "4", "Use UTF-8 in TextEdit"},

	{"com.apple.TimeMachine", "DoNotOfferNewDisksForBackup", "bool", "true", "Don't prompt for Time Machine on new disks"},
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

func Configure(prefs []Preference, dryRun bool) error {
	for _, pref := range prefs {
		value := expandHome(pref.Value)

		if dryRun {
			fmt.Printf("[DRY-RUN] Would set %s %s = %s (%s)\n", pref.Domain, pref.Key, value, pref.Desc)
			continue
		}

		args := []string{"write", pref.Domain, pref.Key}
		switch pref.Type {
		case "bool":
			args = append(args, "-bool", value)
		case "int":
			args = append(args, "-int", value)
		case "float":
			args = append(args, "-float", value)
		case "string":
			args = append(args, "-string", value)
		default:
			args = append(args, value)
		}

		cmd := exec.Command("defaults", args...)
		if err := cmd.Run(); err != nil {
			fmt.Printf("Warning: failed to set %s %s: %v\n", pref.Domain, pref.Key, err)
		}
	}

	return nil
}

func CreateScreenshotsDir(dryRun bool) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot determine home directory: %w", err)
	}
	dir := filepath.Join(home, "Screenshots")

	if dryRun {
		fmt.Printf("[DRY-RUN] Would create %s directory\n", dir)
		return nil
	}

	return os.MkdirAll(dir, 0755)
}

func RestartAffectedApps(dryRun bool) error {
	apps := []string{"Finder", "Dock", "SystemUIServer"}

	for _, app := range apps {
		if dryRun {
			fmt.Printf("[DRY-RUN] Would restart %s\n", app)
			continue
		}

		// killall returns an error if the app isn't running, which is expected and safe to ignore
		cmd := exec.Command("killall", app)
		cmd.Run() //nolint:errcheck // non-fatal: app may not be running
	}

	return nil
}
