package search

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/openbootdotdev/openboot/internal/config"
	"github.com/openbootdotdev/openboot/internal/system"
)

var httpClient = &http.Client{
	Timeout: 8 * time.Second,
	Transport: &http.Transport{
		ForceAttemptHTTP2: false,
	},
}

func getAPIBase() string {
	if base := os.Getenv("OPENBOOT_API_URL"); base != "" {
		if system.IsAllowedAPIURL(base) {
			return base + "/api"
		}
	}
	return "https://openboot.dev/api"
}

type searchResult struct {
	Name string `json:"name"`
	Desc string `json:"desc"`
	Type string `json:"type"`
}

type searchResponse struct {
	Results []searchResult `json:"results"`
}

func queryAPI(endpoint, query string) ([]config.Package, error) {
	u := fmt.Sprintf("%s/%s/search?q=%s", getAPIBase(), endpoint, url.QueryEscape(query))
	resp, err := httpClient.Get(u)
	if err != nil {
		return nil, fmt.Errorf("%s search: %w", endpoint, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read %s body: %w", endpoint, err)
	}

	var sr searchResponse
	if err := json.Unmarshal(body, &sr); err != nil {
		return nil, fmt.Errorf("parse %s: %w", endpoint, err)
	}

	var pkgs []config.Package
	for _, r := range sr.Results {
		pkg := config.Package{
			Name:        r.Name,
			Description: r.Desc,
		}
		switch r.Type {
		case "cask":
			pkg.IsCask = true
		case "npm":
			pkg.IsNpm = true
		}
		pkgs = append(pkgs, pkg)
	}
	return pkgs, nil
}

func SearchOnline(query string) ([]config.Package, error) {
	if query == "" {
		return nil, nil
	}

	type result struct {
		pkgs []config.Package
		err  error
	}

	brewCh := make(chan result, 1)
	npmCh := make(chan result, 1)

	go func() {
		pkgs, err := queryAPI("homebrew", query)
		brewCh <- result{pkgs, err}
	}()

	go func() {
		pkgs, err := queryAPI("npm", query)
		npmCh <- result{pkgs, err}
	}()

	var all []config.Package
	var firstErr error

	br := <-brewCh
	if br.err != nil && firstErr == nil {
		firstErr = br.err
	}
	all = append(all, br.pkgs...)

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
