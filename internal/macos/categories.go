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
			// View
			{"com.apple.finder", "ShowPathbar", "bool", "true", "Show path bar in Finder"},
			{"com.apple.finder", "ShowStatusBar", "bool", "true", "Show status bar in Finder"},
			{"com.apple.finder", "ShowSidebar", "bool", "true", "Show sidebar in Finder"},
			{"com.apple.finder", "ShowTabView", "bool", "true", "Show tab bar in Finder"},
			{"com.apple.finder", "ShowPreviewPane", "bool", "false", "Show preview pane in Finder"},
			{"com.apple.finder", "FXPreferredViewStyle", "string", "Nlsv", "Use list view in Finder"},
			{"com.apple.finder", "AppleShowAllFiles", "bool", "true", "Show hidden files in Finder"},
			{"com.apple.finder", "_FXShowPosixPathInTitle", "bool", "true", "Show full POSIX path in Finder title"},
			// Behavior
			{"com.apple.finder", "FXEnableExtensionChangeWarning", "bool", "false", "No extension change warning"},
			{"com.apple.finder", "FXDefaultSearchScope", "string", "SCcf", "Search current folder by default"},
			{"com.apple.finder", "_FXSortFoldersFirst", "bool", "true", "Keep folders on top when sorting by name"},
			{"com.apple.finder", "_FXSortFoldersFirstOnDesktop", "bool", "true", "Keep folders on top on Desktop"},
			{"com.apple.finder", "WarnOnEmptyTrash", "bool", "false", "Don't warn before emptying Trash"},
			{"com.apple.finder", "FXRemoveOldTrashItems", "bool", "true", "Remove Trash items after 30 days"},
			{"com.apple.finder", "QuitMenuItem", "bool", "true", "Allow quitting Finder via ⌘Q"},
			// New Window
			{"com.apple.finder", "NewWindowTarget", "string", "PfHm", "New Finder window target (PfHm=Home, PfDe=Desktop, PfDo=Documents, PfLo=Other)"},
			{"com.apple.finder", "NewWindowTargetPath", "string", "", "Custom path for new Finder windows (when target is PfLo)"},
			// Desktop icons
			{"com.apple.finder", "ShowExternalHardDrivesOnDesktop", "bool", "true", "Show external drives on Desktop"},
			{"com.apple.finder", "ShowHardDrivesOnDesktop", "bool", "false", "Don't show hard drives on Desktop"},
			{"com.apple.finder", "ShowMountedServersOnDesktop", "bool", "false", "Don't show servers on Desktop"},
			{"com.apple.finder", "ShowRemovableMediaOnDesktop", "bool", "true", "Show removable media on Desktop"},
			{"com.apple.finder", "ShowRecentTags", "bool", "true", "Show recent tags in sidebar"},
			// .DS_Store
			{"com.apple.desktopservices", "DSDontWriteNetworkStores", "bool", "true", "No .DS_Store on network volumes"},
			{"com.apple.desktopservices", "DSDontWriteUSBStores", "bool", "true", "No .DS_Store on USB volumes"},
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
		Name: "Trackpad",
		Icon: "🖱",
		Prefs: []Preference{
			{"com.apple.AppleMultitouchTrackpad", "Clicking", "bool", "true", "Tap to click"},
			{"com.apple.driver.AppleBluetoothMultitouch.trackpad", "Clicking", "bool", "true", "Tap to click (Bluetooth)"},
			{"com.apple.AppleMultitouchTrackpad", "TrackpadThreeFingerDrag", "bool", "true", "Three-finger drag"},
		},
	},
	{
		Name: "Keyboard",
		Icon: "⌨",
		Prefs: []Preference{
			{"NSGlobalDomain", "com.apple.keyboard.fnState", "bool", "true", "Use F1, F2, etc. as standard function keys"},
		},
	},
	{
		Name: "Mission Control",
		Icon: "🖥",
		Prefs: []Preference{
			{"com.apple.dock", "mru-spaces", "bool", "false", "Don't auto-rearrange Spaces based on recent use"},
			{"com.apple.dock", "expose-group-apps", "bool", "true", "Group windows by application"},
		},
	},
	{
		Name: "Security",
		Icon: "🔒",
		Prefs: []Preference{
			{"com.apple.screensaver", "askForPassword", "int", "1", "Require password after sleep or screen saver"},
			{"com.apple.screensaver", "askForPasswordDelay", "int", "0", "No delay before password required"},
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
