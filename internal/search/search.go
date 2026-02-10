package search

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/openbootdotdev/openboot/internal/config"
)

// httpClient uses HTTP/1.1 explicitly to avoid EOF issues with Cloudflare/CDN endpoints.
var httpClient = &http.Client{
	Timeout: 10 * time.Second,
	Transport: &http.Transport{
		ForceAttemptHTTP2: false,
	},
}

var (
	brewFormulaCache  []brewFormula
	brewFormulaMu     sync.Mutex
	brewFormulaLoaded bool

	brewCaskCache  []brewCask
	brewCaskMu     sync.Mutex
	brewCaskLoaded bool
)

type brewFormula struct {
	Name string `json:"name"`
	Desc string `json:"desc"`
}

type brewCask struct {
	Token string   `json:"token"`
	Name  []string `json:"name"`
	Desc  string   `json:"desc"`
}

func fetchBrewFormulae() ([]brewFormula, error) {
	brewFormulaMu.Lock()
	defer brewFormulaMu.Unlock()

	if brewFormulaLoaded {
		return brewFormulaCache, nil
	}

	resp, err := httpClient.Get("https://formulae.brew.sh/api/formula.json")
	if err != nil {
		return nil, fmt.Errorf("fetch formulae: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read formulae body: %w", err)
	}

	var formulae []brewFormula
	if err := json.Unmarshal(body, &formulae); err != nil {
		return nil, fmt.Errorf("parse formulae: %w", err)
	}

	brewFormulaCache = formulae
	brewFormulaLoaded = true
	return brewFormulaCache, nil
}

func fetchBrewCasks() ([]brewCask, error) {
	brewCaskMu.Lock()
	defer brewCaskMu.Unlock()

	if brewCaskLoaded {
		return brewCaskCache, nil
	}

	resp, err := httpClient.Get("https://formulae.brew.sh/api/cask.json")
	if err != nil {
		return nil, fmt.Errorf("fetch casks: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read casks body: %w", err)
	}

	var casks []brewCask
	if err := json.Unmarshal(body, &casks); err != nil {
		return nil, fmt.Errorf("parse casks: %w", err)
	}

	brewCaskCache = casks
	brewCaskLoaded = true
	return brewCaskCache, nil
}

type npmSearchResponse struct {
	Objects []struct {
		Package struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"package"`
	} `json:"objects"`
}

func searchNpm(query string) ([]config.Package, error) {
	url := fmt.Sprintf("https://registry.npmjs.org/-/v1/search?text=%s&size=10", query)
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("npm search: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read npm body: %w", err)
	}

	var result npmSearchResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse npm: %w", err)
	}

	var pkgs []config.Package
	for _, obj := range result.Objects {
		pkgs = append(pkgs, config.Package{
			Name:        obj.Package.Name,
			Description: obj.Package.Description,
			IsNpm:       true,
		})
	}
	return pkgs, nil
}

// SearchOnline queries Homebrew formulae, casks, and npm concurrently.
// Homebrew lists are cached in memory after first fetch. Safe for concurrent use.
func SearchOnline(query string) ([]config.Package, error) {
	if query == "" {
		return nil, nil
	}

	lowerQuery := strings.ToLower(query)

	type result struct {
		pkgs []config.Package
		err  error
	}

	formulaeCh := make(chan result, 1)
	casksCh := make(chan result, 1)
	npmCh := make(chan result, 1)

	go func() {
		formulae, err := fetchBrewFormulae()
		if err != nil {
			formulaeCh <- result{nil, err}
			return
		}
		var matched []config.Package
		for _, f := range formulae {
			if strings.Contains(strings.ToLower(f.Name), lowerQuery) ||
				strings.Contains(strings.ToLower(f.Desc), lowerQuery) {
				matched = append(matched, config.Package{
					Name:        f.Name,
					Description: f.Desc,
				})
			}
			if len(matched) >= 10 {
				break
			}
		}
		formulaeCh <- result{matched, nil}
	}()

	go func() {
		casks, err := fetchBrewCasks()
		if err != nil {
			casksCh <- result{nil, err}
			return
		}
		var matched []config.Package
		for _, c := range casks {
			nameMatch := strings.Contains(strings.ToLower(c.Token), lowerQuery)
			descMatch := strings.Contains(strings.ToLower(c.Desc), lowerQuery)
			if nameMatch || descMatch {
				matched = append(matched, config.Package{
					Name:        c.Token,
					Description: c.Desc,
					IsCask:      true,
				})
			}
			if len(matched) >= 10 {
				break
			}
		}
		casksCh <- result{matched, nil}
	}()

	go func() {
		pkgs, err := searchNpm(query)
		npmCh <- result{pkgs, err}
	}()

	var all []config.Package
	var firstErr error

	fr := <-formulaeCh
	if fr.err != nil && firstErr == nil {
		firstErr = fr.err
	}
	all = append(all, fr.pkgs...)

	cr := <-casksCh
	if cr.err != nil && firstErr == nil {
		firstErr = cr.err
	}
	all = append(all, cr.pkgs...)

	nr := <-npmCh
	if nr.err != nil && firstErr == nil {
		firstErr = nr.err
	}
	all = append(all, nr.pkgs...)

	if len(all) > 0 {
		return all, nil
	}
	return all, firstErr
}
