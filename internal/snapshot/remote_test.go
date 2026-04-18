package snapshot

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeSnapshotFile(t *testing.T, snap *Snapshot) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "snap.json")
	data, err := json.Marshal(snap)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0600))
	return path
}

func TestLoadFromSource_LocalFile(t *testing.T) {
	path := writeSnapshotFile(t, &Snapshot{
		CapturedAt: time.Now().UTC(),
		Packages:   PackageSnapshot{Formulae: []string{"git"}},
	})

	snap, err := LoadFromSource(context.Background(), path)
	require.NoError(t, err)
	require.NotNil(t, snap)
	assert.Equal(t, []string{"git"}, snap.Packages.Formulae)
}

func TestLoadFromSource_RejectsHTTP(t *testing.T) {
	_, err := LoadFromSource(context.Background(), "http://example.com/snap.json")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "insecure HTTP not allowed")
}

func TestLoadFromSource_HTTPSDownloadsAndParses(t *testing.T) {
	snap := &Snapshot{
		CapturedAt: time.Now().UTC(),
		Packages:   PackageSnapshot{Formulae: []string{"git", "go"}},
	}
	body, err := json.Marshal(snap)
	require.NoError(t, err)

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer server.Close()

	// Use server's client so the test trusts the TLS cert.
	oldDefault := http.DefaultTransport
	http.DefaultTransport = server.Client().Transport
	t.Cleanup(func() { http.DefaultTransport = oldDefault })

	got, err := LoadFromSource(context.Background(), server.URL)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, []string{"git", "go"}, got.Packages.Formulae)
}

func TestLoadFromSource_HTTPSNon200(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	oldDefault := http.DefaultTransport
	http.DefaultTransport = server.Client().Transport
	t.Cleanup(func() { http.DefaultTransport = oldDefault })

	_, err := LoadFromSource(context.Background(), server.URL)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}
