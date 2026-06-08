package snapshot

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseLoginItemsOutput_Empty(t *testing.T) {
	got, err := parseLoginItemsOutput("")
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestParseLoginItemsOutput_TwoItems(t *testing.T) {
	input := "Maccy\t/Applications/Maccy.app\tfalse\n" +
		"BetterDisplay\t/Applications/BetterDisplay.app\ttrue\n"
	got, err := parseLoginItemsOutput(input)
	require.NoError(t, err)
	assert.Equal(t, []LoginItem{
		{Name: "Maccy", Path: "/Applications/Maccy.app", Hidden: false},
		{Name: "BetterDisplay", Path: "/Applications/BetterDisplay.app", Hidden: true},
	}, got)
}

func TestParseLoginItemsOutput_TrailingWhitespace(t *testing.T) {
	input := "Maccy\t/Applications/Maccy.app\tfalse\r\n\n"
	got, err := parseLoginItemsOutput(input)
	require.NoError(t, err)
	assert.Len(t, got, 1)
	assert.Equal(t, "Maccy", got[0].Name)
}

func TestParseLoginItemsOutput_NameWithSpaces(t *testing.T) {
	input := "Scroll Reverser\t/Applications/Scroll Reverser.app\tfalse\n"
	got, err := parseLoginItemsOutput(input)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "Scroll Reverser", got[0].Name)
	assert.Equal(t, "/Applications/Scroll Reverser.app", got[0].Path)
}

func TestParseLoginItemsOutput_MalformedRowSkipped(t *testing.T) {
	input := "OK\t/Applications/OK.app\tfalse\n" +
		"broken row no tabs\n" +
		"Other\t/Applications/Other.app\ttrue\n"
	got, err := parseLoginItemsOutput(input)
	require.NoError(t, err)
	assert.Equal(t, []LoginItem{
		{Name: "OK", Path: "/Applications/OK.app", Hidden: false},
		{Name: "Other", Path: "/Applications/Other.app", Hidden: true},
	}, got)
}

func TestCaptureLoginItems_NoPanic(t *testing.T) {
	items, err := CaptureLoginItems()
	// CaptureLoginItems swallows osascript errors (returns ([]LoginItem{}, nil)
	// in all failure paths) — on CI hosts System Events may deny access.
	// We assert only that the call doesn't panic.
	_ = items
	_ = err
}
