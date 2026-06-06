package snapshot

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPackageSnapshot_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected PackageSnapshot
		wantErr  bool
	}{
		{
			name:  "object format",
			input: `{"formulae":["git","go"],"casks":["docker"],"taps":["homebrew/core"],"npm":["typescript"],"bun":["@anthropic-ai/claude-code"]}`,
			expected: PackageSnapshot{
				Formulae: []string{"git", "go"},
				Casks:    []string{"docker"},
				Taps:     []string{"homebrew/core"},
				Npm:      []string{"typescript"},
				Bun:      []string{"@anthropic-ai/claude-code"},
			},
		},
		{
			name:  "typed object array",
			input: `[{"name":"git","type":"formula"},{"name":"docker","type":"cask"},{"name":"homebrew/core","type":"tap"},{"name":"typescript","type":"npm"},{"name":"prettier","type":"bun"}]`,
			expected: PackageSnapshot{
				Formulae:     []string{"git"},
				Casks:        []string{"docker"},
				Taps:         []string{"homebrew/core"},
				Npm:          []string{"typescript"},
				Bun:          []string{"prettier"},
				Descriptions: map[string]string{},
			},
		},
		{
			name:  "typed object array with desc field",
			input: `[{"name":"ack","type":"formula","desc":"grep for programmers"},{"name":"alfred","type":"cask","desc":"Productivity app"}]`,
			expected: PackageSnapshot{
				Formulae:     []string{"ack"},
				Casks:        []string{"alfred"},
				Descriptions: map[string]string{"ack": "grep for programmers", "alfred": "Productivity app"},
			},
		},
		{
			name:  "flat array treated as formulae",
			input: `["git","curl","jq"]`,
			expected: PackageSnapshot{
				Formulae: []string{"git", "curl", "jq"},
			},
		},
		{
			name:    "invalid type",
			input:   `123`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ps PackageSnapshot
			err := json.Unmarshal([]byte(tt.input), &ps)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expected, ps)
		})
	}
}

func TestPackageSnapshot_MarshalJSON_NoDescriptions(t *testing.T) {
	ps := PackageSnapshot{
		Formulae: []string{"git", "curl"},
		Casks:    []string{"docker"},
		Taps:     []string{"homebrew/core"},
		Npm:      []string{"typescript"},
	}

	data, err := json.Marshal(ps)
	require.NoError(t, err)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &raw))

	var formulae []string
	require.NoError(t, json.Unmarshal(raw["formulae"], &formulae))
	assert.Equal(t, []string{"git", "curl"}, formulae)
}

func TestPackageSnapshot_MarshalJSON_AlwaysPlainStrings(t *testing.T) {
	// MarshalJSON always outputs plain string arrays (canonical format).
	// Descriptions are runtime-only and not serialised.
	ps := PackageSnapshot{
		Formulae: []string{"git", "curl"},
		Casks:    []string{"docker"},
		Taps:     []string{"homebrew/core"},
		Npm:      []string{"typescript"},
		Descriptions: map[string]string{
			"git":        "Version control system",
			"curl":       "Transfer data with URLs",
			"docker":     "Container platform",
			"typescript": "Typed JavaScript",
		},
	}

	data, err := json.Marshal(ps)
	require.NoError(t, err)

	var raw struct {
		Formulae []string `json:"formulae"`
		Casks    []string `json:"casks"`
		Taps     []string `json:"taps"`
		Npm      []string `json:"npm"`
	}
	require.NoError(t, json.Unmarshal(data, &raw))

	assert.Equal(t, []string{"git", "curl"}, raw.Formulae)
	assert.Equal(t, []string{"docker"}, raw.Casks)
	assert.Equal(t, []string{"homebrew/core"}, raw.Taps)
	assert.Equal(t, []string{"typescript"}, raw.Npm)
}

func TestPackageSnapshot_MarshalJSON_RoundTrip(t *testing.T) {
	// Round-trip preserves package names but not descriptions
	// (descriptions are runtime-only, not serialised).
	tests := []struct {
		name     string
		original PackageSnapshot
	}{
		{
			name: "all package types",
			original: PackageSnapshot{
				Formulae: []string{"git", "curl"},
				Casks:    []string{"docker"},
				Taps:     []string{"homebrew/core"},
				Npm:      []string{"typescript"},
				Bun:      []string{"prettier"},
			},
		},
		{
			name: "cask only",
			original: PackageSnapshot{
				Casks: []string{"docker", "slack"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.original)
			require.NoError(t, err)

			var restored PackageSnapshot
			require.NoError(t, json.Unmarshal(data, &restored))

			assert.Equal(t, tt.original.Formulae, restored.Formulae)
			assert.Equal(t, tt.original.Casks, restored.Casks)
			assert.Equal(t, tt.original.Taps, restored.Taps)
			assert.Equal(t, tt.original.Npm, restored.Npm)
			assert.Equal(t, tt.original.Bun, restored.Bun)
		})
	}
}

func TestMacOSPref_JSON_UnsetRoundTrip(t *testing.T) {
	tests := []struct {
		name       string
		pref       MacOSPref
		wantHasKey bool
		wantValue  bool
	}{
		{
			name:       "set pref omits unset key",
			pref:       MacOSPref{Domain: "d", Key: "k", Type: "bool", Value: "true", Desc: "x"},
			wantHasKey: false,
		},
		{
			name:       "unset pref serializes unset:true",
			pref:       MacOSPref{Domain: "d", Key: "k", Type: "bool", Value: "false", Desc: "x", Unset: true},
			wantHasKey: true,
			wantValue:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.pref)
			require.NoError(t, err)

			var raw map[string]any
			require.NoError(t, json.Unmarshal(data, &raw))

			val, ok := raw["unset"]
			assert.Equal(t, tt.wantHasKey, ok, "unset key presence")
			if ok {
				assert.Equal(t, tt.wantValue, val)
			}

			var back MacOSPref
			require.NoError(t, json.Unmarshal(data, &back))
			assert.Equal(t, tt.pref, back, "round-trip preserves Unset")
		})
	}
}

func TestMacOSPref_JSON_LegacyDecodeNoUnsetField(t *testing.T) {
	// A snapshot written by an older version of openboot will have no
	// "unset" key at all. It must decode cleanly with Unset == false so
	// existing on-disk snapshots keep working.
	legacy := `{"domain":"d","key":"k","type":"bool","value":"true","desc":"x"}`
	var pref MacOSPref
	require.NoError(t, json.Unmarshal([]byte(legacy), &pref))
	assert.False(t, pref.Unset)
}

func TestCaptureHealth(t *testing.T) {
	t.Run("default is healthy", func(t *testing.T) {
		snap := &Snapshot{Version: 1}
		assert.False(t, snap.Health.Partial)
		assert.Empty(t, snap.Health.FailedSteps)
	})

	t.Run("partial when steps fail", func(t *testing.T) {
		snap := &Snapshot{
			Health: CaptureHealth{
				FailedSteps: []string{"Homebrew Formulae", "Homebrew Casks"},
				Partial:     true,
			},
		}
		assert.True(t, snap.Health.Partial)
		assert.Equal(t, 2, len(snap.Health.FailedSteps))
	})
}
