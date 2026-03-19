package macos

// PrefCategory groups related macOS preferences for display in the TUI selector.
type PrefCategory struct {
	Name  string
	Icon  string
	Prefs []Preference
}

// PrefKey returns a unique identifier for a preference, used as the selection map key.
func PrefKey(p Preference) string {
	return p.Domain + "/" + p.Key
}

// DefaultCategories groups DefaultPreferences by logical category.
var DefaultCategories = []PrefCategory{
	{
		Name: "System",
		Icon: "⚙",
		Prefs: []Preference{
			{"NSGlobalDomain", "AppleShowAllExtensions", "bool", "true", "Show all file extensions"},
			{"NSGlobalDomain", "AppleShowScrollBars", "string", "Always", "Always show scrollbars"},
			{"NSGlobalDomain", "NSAutomaticSpellingCorrectionEnabled", "bool", "false", "Disable auto-correct"},
			{"NSGlobalDomain", "NSAutomaticCapitalizationEnabled", "bool", "false", "Disable auto-capitalization"},
			{"NSGlobalDomain", "KeyRepeat", "int", "2", "Fast key repeat rate"},
			{"NSGlobalDomain", "InitialKeyRepeat", "int", "15", "Short delay until key repeat"},
		},
	},
	{
		Name: "Finder",
		Icon: "📁",
		Prefs: []Preference{
			{"com.apple.finder", "ShowPathbar", "bool", "true", "Show path bar in Finder"},
			{"com.apple.finder", "ShowStatusBar", "bool", "true", "Show status bar in Finder"},
			{"com.apple.finder", "FXPreferredViewStyle", "string", "Nlsv", "Use list view in Finder"},
			{"com.apple.finder", "FXEnableExtensionChangeWarning", "bool", "false", "No extension change warning"},
			{"com.apple.finder", "AppleShowAllFiles", "bool", "true", "Show hidden files in Finder"},
		},
	},
	{
		Name: "Dock",
		Icon: "🚢",
		Prefs: []Preference{
			{"com.apple.dock", "autohide", "bool", "false", "Keep Dock visible"},
			{"com.apple.dock", "show-recents", "bool", "false", "Don't show recent apps in Dock"},
			{"com.apple.dock", "tilesize", "int", "48", "Set Dock icon size to 48"},
			{"com.apple.dock", "mineffect", "string", "scale", "Minimize windows with scale effect"},
		},
	},
	{
		Name: "Screenshots",
		Icon: "📸",
		Prefs: []Preference{
			{"com.apple.screencapture", "location", "string", "~/Screenshots", "Save screenshots to ~/Screenshots"},
			{"com.apple.screencapture", "type", "string", "png", "Save screenshots as PNG"},
			{"com.apple.screencapture", "disable-shadow", "bool", "true", "Disable screenshot shadows"},
		},
	},
	{
		Name: "Safari",
		Icon: "🌐",
		Prefs: []Preference{
			{"com.apple.Safari", "IncludeDevelopMenu", "bool", "true", "Enable Safari Developer menu"},
			{"com.apple.Safari", "WebKitDeveloperExtrasEnabledPreferenceKey", "bool", "true", "Enable Safari WebKit dev extras"},
		},
	},
	{
		Name: "TextEdit",
		Icon: "📝",
		Prefs: []Preference{
			{"com.apple.TextEdit", "RichText", "bool", "false", "Use plain text in TextEdit"},
			{"com.apple.TextEdit", "PlainTextEncoding", "int", "4", "Use UTF-8 in TextEdit"},
		},
	},
	{
		Name: "TimeMachine",
		Icon: "💾",
		Prefs: []Preference{
			{"com.apple.TimeMachine", "DoNotOfferNewDisksForBackup", "bool", "true", "Don't prompt for Time Machine on new disks"},
		},
	},
}

// AllPrefsSelected returns a map with all default preferences set to true.
func AllPrefsSelected() map[string]bool {
	selected := make(map[string]bool)
	for _, cat := range DefaultCategories {
		for _, p := range cat.Prefs {
			selected[PrefKey(p)] = true
		}
	}
	return selected
}
