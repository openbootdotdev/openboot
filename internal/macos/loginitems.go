package macos

import (
	"fmt"
	"os"
	"strings"

	"github.com/openbootdotdev/openboot/internal/system"
)

// LoginItem represents one entry in the user's Login Items list.
// Name is the System Events identifier; Path is the absolute path to the
// .app bundle; Hidden controls whether the app launches hidden.
type LoginItem struct {
	Name   string
	Path   string
	Hidden bool
}

// SetLoginItems replaces the user's Login Items list with the given
// items, in order. Declarative-replace semantics: the leading "delete
// every login item" pass removes anything present on the system that
// isn't in the input, then each input item is recreated (so existing
// items with matching names are reset to the new path/hidden state).
//
// Items whose Path does not exist on disk are warned and skipped.
func SetLoginItems(items []LoginItem, dryRun bool) error {
	// Filter unavailable paths up-front so the osascript stays small.
	filtered := make([]LoginItem, 0, len(items))
	for _, it := range items {
		if _, err := os.Stat(it.Path); err != nil {
			if dryRun {
				fmt.Printf("[DRY-RUN] Login items: skip %s (not installed at %s)\n", it.Name, it.Path)
			} else {
				fmt.Fprintf(os.Stderr, "⚠ Login items: skipping %s (not installed at %s)\n", it.Name, it.Path)
			}
			continue
		}
		filtered = append(filtered, it)
	}

	script := loginItemsApplyScript(filtered)

	if dryRun {
		fmt.Println("[DRY-RUN] Would run osascript to replace login items:")
		for _, line := range strings.Split(script, "\n") {
			fmt.Printf("[DRY-RUN]   %s\n", line)
		}
		return nil
	}

	if _, err := system.RunCommandSilent("osascript", "-e", script); err != nil {
		return fmt.Errorf("set login items: %w", err)
	}
	return nil
}

// loginItemsApplyScript returns the osascript that:
//  1. Deletes every existing login item (declarative wipe).
//  2. For each input item, creates a new login item.
//
// Item names and paths are escaped for embedding in a quoted
// AppleScript string by doubling backslashes and escaping double
// quotes per AppleScript convention.
func loginItemsApplyScript(items []LoginItem) string {
	var b strings.Builder
	b.WriteString(`tell application "System Events"` + "\n")
	b.WriteString("\ttry\n")
	b.WriteString("\t\tdelete every login item\n")
	b.WriteString("\tend try\n")
	for _, it := range items {
		fmt.Fprintf(&b,
			"\tmake new login item with properties {name:\"%s\", path:\"%s\", hidden:%t}\n",
			escapeAppleScriptString(it.Name),
			escapeAppleScriptString(it.Path),
			it.Hidden,
		)
	}
	b.WriteString("end tell")
	return b.String()
}

// escapeAppleScriptString escapes double-quotes and backslashes for
// embedding in an AppleScript double-quoted literal.
func escapeAppleScriptString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}
