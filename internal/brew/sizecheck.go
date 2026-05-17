package brew

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/openbootdotdev/openboot/internal/httputil"
)

// caskInfo is the subset of `brew info --json=v2 --cask` we care about.
type caskInfo struct {
	Token string `json:"token"`
	URL   string `json:"url"`
}

// caskInfoEnvelope matches the v2 JSON schema: a top-level object with
// "formulae" and "casks" arrays. (v1 returned a flat top-level array, but
// brew rejects bare `--json` together with `--cask`, so we have to ask for
// v2 explicitly — see https://docs.brew.sh/Manpage `brew info`.)
type caskInfoEnvelope struct {
	Casks []caskInfo `json:"casks"`
}

// FetchCaskSizes resolves each cask's download URL via `brew info --json=v2
// --cask`, then issues HEAD requests in parallel to read Content-Length.
// Returns a map[caskName]bytes; missing entries default to 0 (caller treats
// as "size unknown — no live bytes display").
func FetchCaskSizes(ctx context.Context, casks []string) map[string]int64 {
	result := make(map[string]int64, len(casks))
	if len(casks) == 0 {
		return result
	}

	args := append([]string{"info", "--json=v2", "--cask"}, casks...)
	output, err := currentRunner().Output(args...)
	if err != nil {
		return result
	}

	var env caskInfoEnvelope
	if err := json.Unmarshal(output, &env); err != nil {
		return result
	}
	entries := env.Casks

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

// ghcrAnonymousToken is the well-known token GHCR accepts for anonymous
// public reads — base64 "A". brew uses the same token internally for bottle
// HEAD/GET against ghcr.io.
const ghcrAnonymousToken = "QQ=="

// formulaInfo is the subset of `brew info --json <names>` we care about.
// The v1 default schema returns a top-level array. Bottle URLs are nested
// per-platform; we pick any one — sizes for the same formula are within
// a few percent across platforms, accurate enough for bar progress.
type formulaInfo struct {
	Name   string `json:"name"`
	Bottle struct {
		Stable struct {
			Files map[string]struct {
				URL string `json:"url"`
			} `json:"files"`
		} `json:"stable"`
	} `json:"bottle"`
}

// FetchFormulaSizes resolves each formula's bottle URL via `brew info --json`
// and HEADs it (with the GHCR anonymous Bearer token) to read Content-Length.
// Same fallback as FetchCaskSizes: missing entries default to 0.
func FetchFormulaSizes(ctx context.Context, formulae []string) map[string]int64 {
	result := make(map[string]int64, len(formulae))
	if len(formulae) == 0 {
		return result
	}

	args := append([]string{"info", "--json"}, formulae...)
	output, err := currentRunner().Output(args...)
	if err != nil {
		return result
	}

	var entries []formulaInfo
	if err := json.Unmarshal(output, &entries); err != nil {
		return result
	}

	client := &http.Client{Timeout: 8 * time.Second}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, e := range entries {
		url := pickBottleURL(e)
		if url == "" {
			result[e.Name] = 0
			continue
		}
		wg.Add(1)
		go func(name, url string) {
			defer wg.Done()
			size := headBottleContentLength(ctx, client, url)
			mu.Lock()
			result[name] = size
			mu.Unlock()
		}(e.Name, url)
	}
	wg.Wait()
	return result
}

// pickBottleURL returns any bottle URL from the formula's stable files map.
// Iteration order is unspecified but acceptable: bottles for the same formula
// have near-identical sizes across platforms.
func pickBottleURL(f formulaInfo) string {
	for _, file := range f.Bottle.Stable.Files {
		if file.URL != "" {
			return file.URL
		}
	}
	return ""
}

func headBottleContentLength(ctx context.Context, client *http.Client, url string) int64 {
	resp, err := httputil.HeadWithBearer(ctx, client, url, ghcrAnonymousToken)
	if err != nil {
		return 0
	}
	defer resp.Body.Close() //nolint:errcheck // best-effort body close
	if resp.ContentLength < 0 {
		return 0
	}
	return resp.ContentLength
}
