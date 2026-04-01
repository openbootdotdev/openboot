package brew

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveFormulaNames_EmptyInput(t *testing.T) {
	assert.Empty(t, ResolveFormulaNames(nil))
	assert.Empty(t, ResolveFormulaNames([]string{}))
}

func TestParseFormulaAliases(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		json     interface{}
		expected map[string]string
	}{
		{
			name:  "resolves aliases to canonical names",
			input: []string{"postgresql", "kubectl", "vim"},
			json: []struct {
				Name string `json:"name"`
			}{
				{Name: "postgresql@16"},
				{Name: "kubernetes-cli"},
				{Name: "vim"},
			},
			expected: map[string]string{
				"postgresql": "postgresql@16",
				"kubectl":    "kubernetes-cli",
				"vim":        "vim",
			},
		},
		{
			name:  "falls back to identity when response is shorter than input",
			input: []string{"foo", "bar", "baz"},
			json: []struct {
				Name string `json:"name"`
			}{
				{Name: "foo-canonical"},
			},
			expected: map[string]string{
				"foo": "foo-canonical",
				"bar": "bar",
				"baz": "baz",
			},
		},
		{
			name:  "falls back to identity for empty name fields",
			input: []string{"foo", "bar"},
			json: []struct {
				Name string `json:"name"`
			}{
				{Name: "foo-resolved"},
				{Name: ""},
			},
			expected: map[string]string{
				"foo": "foo-resolved",
				"bar": "bar",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(tc.json)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, parseFormulaAliases(tc.input, data))
		})
	}
}

func TestParseFormulaAliases_InvalidJSON(t *testing.T) {
	names := []string{"pkg1", "pkg2"}
	result := parseFormulaAliases(names, []byte("not json"))
	assert.Equal(t, map[string]string{"pkg1": "pkg1", "pkg2": "pkg2"}, result)
}
