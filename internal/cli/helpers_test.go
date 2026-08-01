package cli

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseConflictError_MaxConfigsMessage(t *testing.T) {
	tests := []struct {
		name            string
		body            []byte
		wantContain     string
		wantAlsoContain string
		wantNotContain  string
	}{
		{
			name:            "message field contains maximum",
			body:            mustMarshalJSON(t, map[string]string{"message": "You have reached the maximum number of configs"}),
			wantContain:     "config limit reached",
			wantAlsoContain: "https://openboot.dev/dashboard",
			wantNotContain:  "openboot delete",
		},
		{
			name:            "error field contains maximum",
			body:            mustMarshalJSON(t, map[string]string{"error": "Maximum configs exceeded"}),
			wantContain:     "config limit reached",
			wantAlsoContain: "https://openboot.dev/dashboard",
			wantNotContain:  "openboot delete",
		},
		{
			name:        "plain message field returned as-is",
			body:        mustMarshalJSON(t, map[string]string{"message": "slug already taken"}),
			wantContain: "slug already taken",
		},
		{
			name:        "error field returned as-is when message empty",
			body:        mustMarshalJSON(t, map[string]string{"error": "invalid slug format"}),
			wantContain: "invalid slug format",
		},
		{
			name:        "unparseable JSON falls back to raw body",
			body:        []byte("not json"),
			wantContain: "conflict: not json",
		},
		{
			name:        "empty JSON object falls back to conflict prefix",
			body:        mustMarshalJSON(t, map[string]string{}),
			wantContain: "conflict:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := parseConflictError(tt.body)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantContain)
			if tt.wantAlsoContain != "" {
				assert.Contains(t, err.Error(), tt.wantAlsoContain)
			}
			if tt.wantNotContain != "" {
				assert.NotContains(t, err.Error(), tt.wantNotContain)
			}
		})
	}
}

func mustMarshalJSON(t *testing.T, v interface{}) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}
