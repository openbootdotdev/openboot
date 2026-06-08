package snapshot

import "strings"

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
