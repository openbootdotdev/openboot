package snapshot

import (
	"strings"

	"github.com/openbootdotdev/openboot/internal/system"
)

// loginItemsScript is the osascript that emits one row per login item
// with name, path, and hidden separated by tab. End-of-row is linefeed.
// Using a separator-based format avoids parsing AppleScript records.
const loginItemsScript = `tell application "System Events"
	set out to ""
	repeat with li in login items
		try
			set out to out & (name of li) & tab & (path of li) & tab & (hidden of li) & linefeed
		end try
	end repeat
	return out
end tell`

// CaptureLoginItems returns the user's currently registered login items.
// Returns ([]LoginItem{}, nil) when none are registered or when System
// Events denies access — capture is best-effort.
func CaptureLoginItems() ([]LoginItem, error) {
	out, err := system.RunCommandOutput("osascript", "-e", loginItemsScript)
	if err != nil {
		return []LoginItem{}, nil
	}
	return parseLoginItemsOutput(out)
}

// parseLoginItemsOutput parses the tab-separated, linefeed-delimited rows
// emitted by the osascript wrapped in CaptureLoginItems. Columns:
//
//	name \t path \t hidden(true|false)
//
// Rows with fewer than three columns are skipped silently — capture is
// best-effort.
func parseLoginItemsOutput(out string) ([]LoginItem, error) {
	out = strings.ReplaceAll(out, "\r", "")
	lines := strings.Split(out, "\n")
	items := make([]LoginItem, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		cols := strings.Split(line, "\t")
		if len(cols) < 3 {
			continue
		}
		items = append(items, LoginItem{
			Name:   cols[0],
			Path:   cols[1],
			Hidden: strings.EqualFold(strings.TrimSpace(cols[2]), "true"),
		})
	}
	return items, nil
}
