package snapshot

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/openbootdotdev/openboot/internal/httputil"
)

// defaultRemoteTimeout bounds HTTP downloads for snapshot imports.
const defaultRemoteTimeout = 30 * time.Second

// maxRemoteSize bounds the number of bytes read from a remote snapshot URL.
const maxRemoteSize int64 = 10 << 20 // 10 MB

// LoadFromSource loads a snapshot from a local path or an HTTPS URL.
// HTTP URLs are rejected. HTTPS URLs are downloaded to a temp file that is
// cleaned up before return. The returned snapshot has CatalogMatch and
// MatchedPreset populated.
func LoadFromSource(ctx context.Context, source string) (*Snapshot, error) {
	localPath := source

	if strings.HasPrefix(source, "http://") {
		return nil, fmt.Errorf("insecure HTTP not allowed for snapshot import — use https:// instead")
	}

	if strings.HasPrefix(source, "https://") {
		var err error
		localPath, err = downloadSnapshot(ctx, source)
		if err != nil {
			return nil, err
		}
		defer os.Remove(localPath)
	}

	snap, err := LoadFile(localPath)
	if err != nil {
		return nil, err
	}

	catalogMatch := MatchPackages(snap)
	snap.CatalogMatch = *catalogMatch
	snap.MatchedPreset = DetectBestPreset(snap)
	return snap, nil
}

// downloadSnapshot fetches a snapshot from a URL into a temp file and
// returns the local path. The caller must remove the file.
func downloadSnapshot(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("download snapshot request: %w", err)
	}

	client := &http.Client{Timeout: defaultRemoteTimeout}
	resp, err := httputil.Do(client, req)
	if err != nil {
		return "", fmt.Errorf("download snapshot: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download snapshot: HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxRemoteSize))
	if err != nil {
		return "", fmt.Errorf("read snapshot response: %w", err)
	}

	tmpFile := filepath.Join(os.TempDir(), "openboot-snapshot-import.json")
	if err := os.WriteFile(tmpFile, data, 0600); err != nil {
		return "", fmt.Errorf("save snapshot: %w", err)
	}
	return tmpFile, nil
}
