package snapshot

import (
	"encoding/xml"
	"fmt"
	"net/url"
	"strings"

	"github.com/openbootdotdev/openboot/internal/system"
)

// CaptureDockApps returns the user's currently pinned Dock apps in order.
// Returns ([]string{}, nil) when the Dock plist has no persistent-apps key.
func CaptureDockApps() ([]string, error) {
	// `defaults export com.apple.dock -` writes the full Dock plist as XML to
	// stdout; piping to `plutil -extract persistent-apps xml1 -o - -` returns
	// just the persistent-apps array as plist XML. We use xml1 (not json)
	// because tile-data contains <data> blobs (alias bookmarks, icon
	// thumbnails) that plutil refuses to serialise as JSON.
	out, err := system.RunCommandOutput(
		"sh", "-c",
		`defaults export com.apple.dock - | plutil -extract persistent-apps xml1 -o - -`,
	)
	if err != nil {
		// Treat as empty rather than fatal — keeps capture lossless
		// when Dock has never been customized.
		return []string{}, nil
	}
	if strings.TrimSpace(out) == "" {
		return []string{}, nil
	}
	return parseDockAppsXML([]byte(out))
}

// parseDockAppsXML extracts absolute app paths from the plist XML output of
//
//	defaults export com.apple.dock - | plutil -extract persistent-apps xml1 -o - -
//
// Non-app tiles (folders, stacks, spacers) are skipped.
// <data> blobs inside tile-data are silently ignored (regression guard).
func parseDockAppsXML(data []byte) ([]string, error) {
	tiles, err := parsePlistArray(data)
	if err != nil {
		return nil, fmt.Errorf("parse dock plist xml: %w", err)
	}

	apps := make([]string, 0, len(tiles))
	for _, tile := range tiles {
		tileType, _ := tile["tile-type"].(string)
		if tileType != "file-tile" {
			continue
		}
		tileData, _ := tile["tile-data"].(plistDict)
		if tileData == nil {
			continue
		}
		fileData, _ := tileData["file-data"].(plistDict)
		if fileData == nil {
			continue
		}
		raw, _ := fileData["_CFURLString"].(string)
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

// plistDict is a map representation of a <dict> element.
type plistDict = map[string]any

// parsePlistArray parses a plist XML document whose root element is <array>
// of <dict> entries and returns them as a slice of plistDict.
func parsePlistArray(data []byte) ([]plistDict, error) {
	dec := xml.NewDecoder(strings.NewReader(string(data)))
	// Advance past the XML declaration, DOCTYPE, and <plist> wrapper to reach
	// the <array> element.
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("reading plist: %w", err)
		}
		if se, ok := tok.(xml.StartElement); ok && se.Name.Local == "array" {
			break
		}
	}
	return readArray(dec)
}

// readArray reads the contents of an already-opened <array> element and
// returns each child as a plistDict. Non-dict children are skipped.
func readArray(dec *xml.Decoder) ([]plistDict, error) {
	var result []plistDict
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("reading array: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "dict" {
				d, err := readDict(dec)
				if err != nil {
					return nil, err
				}
				result = append(result, d)
			} else {
				// Skip any non-dict child (shouldn't appear in dock plist, but be safe).
				if err := dec.Skip(); err != nil {
					return nil, fmt.Errorf("skipping element: %w", err)
				}
			}
		case xml.EndElement:
			// </array>
			return result, nil
		}
	}
}

// readDict reads the contents of an already-opened <dict> element and
// returns it as a plistDict. Values may be strings, integers, dicts, arrays,
// booleans, or data blobs. Data blobs are stored as a sentinel non-nil value
// so callers can detect their presence without caring about the bytes.
func readDict(dec *xml.Decoder) (plistDict, error) {
	d := make(plistDict)
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("reading dict: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local != "key" {
				return nil, fmt.Errorf("expected <key>, got <%s>", t.Name.Local)
			}
			key, err := readCharData(dec)
			if err != nil {
				return nil, fmt.Errorf("reading key: %w", err)
			}
			val, err := readValue(dec)
			if err != nil {
				return nil, fmt.Errorf("reading value for key %q: %w", key, err)
			}
			d[key] = val
		case xml.EndElement:
			// </dict>
			return d, nil
		}
	}
}

// readValue reads the next start element (the value following a <key>) and
// returns a Go representation. The element and its children are consumed.
func readValue(dec *xml.Decoder) (any, error) {
	// Skip CharData (whitespace) before the value element.
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("reading value token: %w", err)
		}
		switch t := tok.(type) {
		case xml.CharData:
			continue
		case xml.StartElement:
			return readValueElement(dec, t)
		case xml.EndElement:
			return nil, fmt.Errorf("unexpected end element <%s> while reading value", t.Name.Local)
		}
	}
}

// readValueElement reads a value given its opening StartElement (already consumed).
func readValueElement(dec *xml.Decoder, se xml.StartElement) (any, error) {
	switch se.Name.Local {
	case "string", "integer", "real":
		return readCharData(dec)
	case "true":
		if err := expectEnd(dec, "true"); err != nil {
			return nil, err
		}
		return true, nil
	case "false":
		if err := expectEnd(dec, "false"); err != nil {
			return nil, err
		}
		return false, nil
	case "data":
		// Consume and discard — we don't need the bytes.
		if err := dec.Skip(); err != nil {
			// dec.Skip() re-consumes including end element, but we already
			// consumed the start element. On error just return a sentinel.
			return "<data>", nil
		}
		// dec.Skip() consumed everything up to and including </data>.
		return "<data>", nil
	case "dict":
		return readDict(dec)
	case "array":
		arr, err := readArray(dec)
		if err != nil {
			return nil, err
		}
		// Convert []plistDict to []any for uniform storage.
		result := make([]any, len(arr))
		for i, d := range arr {
			result[i] = d
		}
		return result, nil
	default:
		// Unknown element — skip it to stay robust.
		if err := dec.Skip(); err != nil {
			return nil, fmt.Errorf("skipping <%s>: %w", se.Name.Local, err)
		}
		return nil, nil
	}
}

// readCharData reads character data up to the next end element and returns
// it as a string. The end element is consumed.
func readCharData(dec *xml.Decoder) (string, error) {
	var buf strings.Builder
	for {
		tok, err := dec.Token()
		if err != nil {
			return "", fmt.Errorf("reading char data: %w", err)
		}
		switch t := tok.(type) {
		case xml.CharData:
			buf.Write(t)
		case xml.EndElement:
			return buf.String(), nil
		}
	}
}

// expectEnd consumes tokens until the matching end element for name is found.
func expectEnd(dec *xml.Decoder, name string) error {
	tok, err := dec.Token()
	if err != nil {
		return fmt.Errorf("expected </%s>: %w", name, err)
	}
	if ee, ok := tok.(xml.EndElement); ok && ee.Name.Local == name {
		return nil
	}
	return fmt.Errorf("expected </%s>, got %T", name, tok)
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
