package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRunEdit_NotAuthenticated(t *testing.T) {
	setupTestAuth(t, false)
	t.Setenv("OPENBOOT_API_URL", "http://localhost:9999")

	err := runEdit("my-config")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not logged in")
}

func TestRunEdit_WithSlugFlag_OpensDirectly(t *testing.T) {
	setupTestAuth(t, true)

	// With --slug, no API call is made (no picker needed). Stub the browser
	// launch so tests don't actually open tabs.
	var capturedURL string
	orig := openBrowser
	openBrowser = func(url string) error { capturedURL = url; return nil }
	t.Cleanup(func() { openBrowser = orig })

	err := runEdit("my-config")
	assert.NoError(t, err)
	assert.Contains(t, capturedURL, "my-config")
}

func TestRunEdit_NoConfigs_ReturnsError(t *testing.T) {
	setupTestAuth(t, true)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"configs": []any{}})
	}))
	defer server.Close()

	t.Setenv("OPENBOOT_API_URL", server.URL)

	err := runEdit("")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no configs found")
}

func TestPickConfig_NoConfigs_ReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"configs": []any{}})
	}))
	defer server.Close()

	_, err := pickConfig("test-token", server.URL)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no configs found")
}

func TestSplitBefore(t *testing.T) {
	assert.Equal(t, "my-setup", splitBefore("my-setup — My Mac Setup", " — "))
	assert.Equal(t, "work-mac", splitBefore("work-mac — Work Machine", " — "))
	assert.Equal(t, "no-sep", splitBefore("no-sep", " — "))
}

func TestEditCmd_CommandStructure(t *testing.T) {
	assert.Equal(t, "edit", editCmd.Use)
	assert.NotEmpty(t, editCmd.Short)
	assert.NotEmpty(t, editCmd.Long)
	assert.NotNil(t, editCmd.RunE)
	assert.NotNil(t, editCmd.Flags().Lookup("slug"))
}
