package snapshot

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// plistHeader is the standard plist XML header used in fixtures below.
const plistHeader = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
`

func TestParseDockAppsXML_Empty(t *testing.T) {
	input := plistHeader + `<array>
</array>
</plist>`
	got, err := parseDockAppsXML([]byte(input))
	require.NoError(t, err)
	assert.Empty(t, got)
}

// TestParseDockAppsXML_TwoApps also serves as the <data>-blob regression
// guard: the "book" key inside tile-data contains a <data> element (a real
// alias bookmark), which previously caused `plutil -extract ... json` to fail
// with "Invalid object in plist for JSON format". The xml1 parser must return
// both app paths without error regardless of the <data> blob.
func TestParseDockAppsXML_TwoApps(t *testing.T) {
	input := plistHeader + `<array>
	<dict>
		<key>GUID</key>
		<integer>4290584577</integer>
		<key>tile-data</key>
		<dict>
			<key>file-data</key>
			<dict>
				<key>_CFURLString</key>
				<string>file:///Applications/Google%20Chrome.app/</string>
				<key>_CFURLStringType</key>
				<integer>15</integer>
			</dict>
			<key>book</key>
			<data>Ym9va0QCAAAAAA==</data>
			<key>bundle-identifier</key>
			<string>com.google.Chrome</string>
		</dict>
		<key>tile-type</key>
		<string>file-tile</string>
	</dict>
	<dict>
		<key>tile-data</key>
		<dict>
			<key>file-data</key>
			<dict>
				<key>_CFURLString</key>
				<string>file:///Applications/Zed.app/</string>
				<key>_CFURLStringType</key>
				<integer>15</integer>
			</dict>
		</dict>
		<key>tile-type</key>
		<string>file-tile</string>
	</dict>
</array>
</plist>`
	got, err := parseDockAppsXML([]byte(input))
	require.NoError(t, err)
	assert.Equal(t, []string{
		"/Applications/Google Chrome.app",
		"/Applications/Zed.app",
	}, got)
}

func TestParseDockAppsXML_NonAppTileSkipped(t *testing.T) {
	input := plistHeader + `<array>
	<dict>
		<key>tile-data</key>
		<dict>
			<key>file-data</key>
			<dict>
				<key>_CFURLString</key>
				<string>file:///Applications/Zed.app/</string>
				<key>_CFURLStringType</key>
				<integer>15</integer>
			</dict>
		</dict>
		<key>tile-type</key>
		<string>file-tile</string>
	</dict>
	<dict>
		<key>tile-data</key>
		<dict>
			<key>arrangement</key>
			<integer>2</integer>
			<key>displayas</key>
			<integer>1</integer>
		</dict>
		<key>tile-type</key>
		<string>directory-tile</string>
	</dict>
</array>
</plist>`
	got, err := parseDockAppsXML([]byte(input))
	require.NoError(t, err)
	assert.Equal(t, []string{"/Applications/Zed.app"}, got)
}

func TestParseDockAppsXML_NonASCIIPath(t *testing.T) {
	input := plistHeader + `<array>
	<dict>
		<key>tile-data</key>
		<dict>
			<key>file-data</key>
			<dict>
				<key>_CFURLString</key>
				<string>file:///Applications/%E5%BE%AE%E4%BF%A1.app/</string>
				<key>_CFURLStringType</key>
				<integer>15</integer>
			</dict>
		</dict>
		<key>tile-type</key>
		<string>file-tile</string>
	</dict>
</array>
</plist>`
	got, err := parseDockAppsXML([]byte(input))
	require.NoError(t, err)
	assert.Equal(t, []string{"/Applications/微信.app"}, got)
}

func TestParseDockAppsXML_DataBlobRegression(t *testing.T) {
	// Regression guard: a <data> blob anywhere inside tile-data must not
	// break parsing. Previously, plutil's json output path failed with
	// "Invalid object in plist for JSON format" when such blobs existed,
	// causing CaptureDockApps to silently return an empty slice.
	// This test is the canonical proof that the xml1 path handles it.
	input := plistHeader + `<array>
	<dict>
		<key>tile-data</key>
		<dict>
			<key>file-data</key>
			<dict>
				<key>_CFURLString</key>
				<string>file:///Applications/Ghostty.app/</string>
				<key>_CFURLStringType</key>
				<integer>15</integer>
			</dict>
			<key>book</key>
			<data>
				Ym9va0QCAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
				AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=
			</data>
			<key>dock-extra</key>
			<false/>
		</dict>
		<key>tile-type</key>
		<string>file-tile</string>
	</dict>
</array>
</plist>`
	got, err := parseDockAppsXML([]byte(input))
	require.NoError(t, err)
	assert.Equal(t, []string{"/Applications/Ghostty.app"}, got)
}

func TestParseDockAppsXML_MalformedXML(t *testing.T) {
	_, err := parseDockAppsXML([]byte(`not xml at all`))
	assert.Error(t, err)
}

// TestCaptureDockApps_NoPanic guards against panic regardless of whether
// the Dock plist exists / has entries on the host running tests.
func TestCaptureDockApps_NoPanic(t *testing.T) {
	apps, err := CaptureDockApps()
	// CaptureDockApps swallows subprocess errors (returns ([]string{}, nil)
	// in all failure paths) so capture stays lossless on virgin machines.
	// We assert only that the call doesn't panic.
	_ = apps
	_ = err
}
