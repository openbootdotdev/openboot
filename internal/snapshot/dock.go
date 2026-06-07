package snapshot

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// dockTile is the subset of a persistent-apps entry we care about.
type dockTile struct {
	TileType string `json:"tile-type"`
	TileData struct {
		FileData struct {
			CFURLString string `json:"_CFURLString"`
		} `json:"file-data"`
	} `json:"tile-data"`
}

// parseDockAppsJSON extracts absolute app paths from the JSON output of
// `defaults export com.apple.dock - | plutil -extract persistent-apps json -o - -`.
// Non-app tiles (folders, stacks, spacers) are skipped per spec.
func parseDockAppsJSON(data []byte) ([]string, error) {
	var tiles []dockTile
	if err := json.Unmarshal(data, &tiles); err != nil {
		return nil, fmt.Errorf("parse dock plist json: %w", err)
	}
	apps := make([]string, 0, len(tiles))
	for _, t := range tiles {
		if t.TileType != "file-tile" {
			continue
		}
		raw := t.TileData.FileData.CFURLString
		if raw == "" {
			continue
		}
		path, err := dockURLToPath(raw)
		if err != nil {
			continue
		}
		apps = append(apps, path)
	}
	return apps, nil
}

// dockURLToPath converts a `file:///Applications/Foo.app/` URL into the
// filesystem path `/Applications/Foo.app`.
func dockURLToPath(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if u.Scheme != "file" {
		return "", fmt.Errorf("expected file:// url, got %q", u.Scheme)
	}
	p, err := url.PathUnescape(u.Path)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(p, "/"), nil
}
