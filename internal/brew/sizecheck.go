package brew

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/openbootdotdev/openboot/internal/httputil"
)

// caskInfo is the subset of `brew info --json --cask` we care about.
type caskInfo struct {
	Token string `json:"token"`
	URL   string `json:"url"`
}

// FetchCaskSizes resolves each cask's download URL via `brew info --json
// --cask`, then issues HEAD requests in parallel to read Content-Length.
// Returns a map[caskName]bytes; missing entries default to 0 (caller treats
// as "size unknown — no live bytes display").
func FetchCaskSizes(ctx context.Context, casks []string) map[string]int64 {
	result := make(map[string]int64, len(casks))
	if len(casks) == 0 {
		return result
	}

	args := append([]string{"info", "--json", "--cask"}, casks...)
	output, err := currentRunner().Output(args...)
	if err != nil {
		return result
	}

	var entries []caskInfo
	if err := json.Unmarshal(output, &entries); err != nil {
		return result
	}

	client := &http.Client{Timeout: 8 * time.Second}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, e := range entries {
		if e.URL == "" {
			result[e.Token] = 0
			continue
		}
		wg.Add(1)
		go func(token, url string) {
			defer wg.Done()
			size := headContentLength(ctx, client, url)
			mu.Lock()
			result[token] = size
			mu.Unlock()
		}(e.Token, e.URL)
	}
	wg.Wait()
	return result
}

func headContentLength(ctx context.Context, client *http.Client, url string) int64 {
	resp, err := httputil.Head(ctx, client, url)
	if err != nil {
		return 0
	}
	defer resp.Body.Close() //nolint:errcheck // best-effort body close
	if resp.ContentLength < 0 {
		return 0
	}
	return resp.ContentLength
}
