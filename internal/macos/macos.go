package macos

import (
	"fmt"
	"os/exec"
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

	{"com.apple.dock", "autohide", "bool", "true", "Auto-hide Dock"},
	{"com.apple.dock", "autohide-delay", "float", "0", "No Dock auto-hide delay"},
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

func Configure(prefs []Preference, dryRun bool) error {
	for _, pref := range prefs {
		if dryRun {
			fmt.Printf("[DRY-RUN] Would set %s %s = %s (%s)\n", pref.Domain, pref.Key, pref.Value, pref.Desc)
			continue
		}

		args := []string{"write", pref.Domain, pref.Key}
		switch pref.Type {
		case "bool":
			args = append(args, "-bool", pref.Value)
		case "int":
			args = append(args, "-int", pref.Value)
		case "float":
			args = append(args, "-float", pref.Value)
		case "string":
			args = append(args, "-string", pref.Value)
		default:
			args = append(args, pref.Value)
		}

		cmd := exec.Command("defaults", args...)
		if err := cmd.Run(); err != nil {
			fmt.Printf("Warning: failed to set %s %s: %v\n", pref.Domain, pref.Key, err)
		}
	}

	return nil
}

func CreateScreenshotsDir(dryRun bool) error {
	if dryRun {
		fmt.Println("[DRY-RUN] Would create ~/Screenshots directory")
		return nil
	}

	cmd := exec.Command("mkdir", "-p", "$HOME/Screenshots")
	cmd.Run()
	return nil
}

func RestartAffectedApps(dryRun bool) error {
	apps := []string{"Finder", "Dock", "SystemUIServer"}

	for _, app := range apps {
		if dryRun {
			fmt.Printf("[DRY-RUN] Would restart %s\n", app)
			continue
		}

		cmd := exec.Command("killall", app)
		cmd.Run()
	}

	return nil
}
