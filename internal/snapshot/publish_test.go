package snapshot

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSlugToTitle(t *testing.T) {
	tests := []struct {
		in  string
		out string
	}{
		{"my-mac-setup", "My Mac Setup"},
		{"foo", "Foo"},
		{"", ""},
		{"a-b", "A B"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			assert.Equal(t, tt.out, SlugToTitle(tt.in))
		})
	}
}

func TestPublish_RequiresSnapshot(t *testing.T) {
	_, err := Publish(context.Background(), PublishOptions{
		Token:   "t",
		APIBase: "http://example.com",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "snapshot")
}

func TestPublish_CreatePOSTWithoutSlug(t *testing.T) {
	var gotMethod string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		_ = json.NewEncoder(w).Encode(map[string]any{"slug": "new"})
	}))
	defer server.Close()

	result, err := Publish(context.Background(), PublishOptions{
		Snapshot:    &Snapshot{},
		Name:        "My Setup",
		Description: "desc",
		Visibility:  "public",
		Token:       "t",
		APIBase:     server.URL,
	})
	require.NoError(t, err)
	assert.Equal(t, "new", result.Slug)
	assert.False(t, result.Updated)
	assert.Equal(t, http.MethodPost, gotMethod)
	assert.NotContains(t, gotBody, "config_slug")
}

func TestPublish_UpdatePUTWithSlug(t *testing.T) {
	var gotMethod string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		_ = json.NewEncoder(w).Encode(map[string]any{"slug": "mine"})
	}))
	defer server.Close()

	result, err := Publish(context.Background(), PublishOptions{
		Snapshot: &Snapshot{},
		Slug:     "mine",
		Token:    "t",
		APIBase:  server.URL,
	})
	require.NoError(t, err)
	assert.Equal(t, "mine", result.Slug)
	assert.True(t, result.Updated)
	assert.Equal(t, http.MethodPut, gotMethod)
	assert.Equal(t, "mine", gotBody["config_slug"])
}

func TestPublish_409MaximumMapsToFriendlyError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]any{"message": "maximum configs reached"})
	}))
	defer server.Close()

	_, err := Publish(context.Background(), PublishOptions{
		Snapshot: &Snapshot{},
		Token:    "t",
		APIBase:  server.URL,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "config limit reached")
}

func TestPublish_409MessagePassedThroughWhenNotMaximum(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]any{"message": "slug taken"})
	}))
	defer server.Close()

	_, err := Publish(context.Background(), PublishOptions{
		Snapshot: &Snapshot{},
		Token:    "t",
		APIBase:  server.URL,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "slug taken")
}

func TestPublish_EmptyResponseSlugFallsBackToOptsSlug(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{})
	}))
	defer server.Close()

	result, err := Publish(context.Background(), PublishOptions{
		Snapshot: &Snapshot{},
		Slug:     "fallback",
		Token:    "t",
		APIBase:  server.URL,
	})
	require.NoError(t, err)
	assert.Equal(t, "fallback", result.Slug)
}
