package config

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// remotePackage matches the JSON returned by GET /api/packages.
type remotePackage struct {
	Name      string `json:"name"`
	Desc      string `json:"desc"`
	Category  string `json:"category"`
	Type      string `json:"type"`
	Installer string `json:"installer"` // "formula", "cask", or "npm"
}

type remotePackagesResponse struct {
	Packages []remotePackage `json:"packages"`
}

const (
	packagesCacheFile = "packages-cache.json"
	packagesCacheTTL  = 24 * time.Hour
)

// packagesCacheEntry is the on-disk cache format.
type packagesCacheEntry struct {
	FetchedAt time.Time       `json:"fetched_at"`
	Packages  []remotePackage `json:"packages"`
}

// RefreshPackagesFromRemote fetches packages from the server and merges them
// into the global Categories slice. Safe to call multiple times; it is a no-op
// if the cache is fresh. Falls back to the embedded packages.yaml silently.
func RefreshPackagesFromRemote() {
	pkgs, err := loadRemotePackages()
	if err != nil || len(pkgs) == 0 {
		return // keep embedded fallback
	}
	mergeRemotePackages(pkgs)
}

func loadRemotePackages() ([]remotePackage, error) {
	// Try disk cache first.
	if pkgs, err := readPackagesCache(); err == nil {
		return pkgs, nil
	}

	// Fetch from server.
	pkgs, err := fetchRemotePackages()
	if err != nil {
		return nil, err
	}

	// Write cache (best-effort).
	_ = writePackagesCache(pkgs)
	return pkgs, nil
}

func fetchRemotePackages() ([]remotePackage, error) {
	apiURL := getAPIBase() + "/api/packages"
	client := &http.Client{Timeout: 8 * time.Second}

	resp, err := client.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("fetch packages: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("fetch packages: status %d", resp.StatusCode)
	}

	var result remotePackagesResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result); err != nil {
		return nil, fmt.Errorf("parse packages: %w", err)
	}

	return result.Packages, nil
}

// cacheDir returns the directory for cache files. It is a variable so tests
// can replace it with a temp directory.
var cacheDir = func() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".openboot")
}

func readPackagesCache() ([]remotePackage, error) {
	data, err := os.ReadFile(filepath.Join(cacheDir(), packagesCacheFile))
	if err != nil {
		return nil, err
	}

	var entry packagesCacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, err
	}

	if time.Since(entry.FetchedAt) > packagesCacheTTL {
		return nil, fmt.Errorf("cache expired")
	}

	return entry.Packages, nil
}

func writePackagesCache(pkgs []remotePackage) error {
	dir := cacheDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	entry := packagesCacheEntry{
		FetchedAt: time.Now(),
		Packages:  pkgs,
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(dir, packagesCacheFile), data, 0600)
}

// categoryMap maps server category names to display info.
var categoryMap = map[string]struct {
	Name string
	Icon string
}{
	"essential":    {Name: "Essential", Icon: "⚡"},
	"development":  {Name: "Development", Icon: "🛠"},
	"productivity": {Name: "Productivity", Icon: "🚀"},
	"optional":     {Name: "Optional", Icon: "📦"},
}

// mergeRemotePackages converts remote packages into Categories format and
// merges them with the embedded data. Remote packages take precedence.
func mergeRemotePackages(pkgs []remotePackage) {
	// Build a set of existing package names from embedded data.
	existing := make(map[string]bool)
	for _, cat := range Categories {
		for _, pkg := range cat.Packages {
			existing[pkg.Name] = true
		}
	}

	// Group new remote packages by category.
	byCat := make(map[string][]Package)
	for _, rp := range pkgs {
		if existing[rp.Name] {
			// Update description in existing categories if remote has a better one.
			updateDescription(rp.Name, rp.Desc)
			// Update installer type flags.
			updateInstallerFlags(rp.Name, rp.Installer)
			continue
		}
		pkg := Package{
			Name:        rp.Name,
			Description: rp.Desc,
			IsCask:      rp.Installer == "cask",
			IsNpm:       rp.Installer == "npm",
		}
		byCat[rp.Category] = append(byCat[rp.Category], pkg)
	}

	// Append new packages to existing categories or create new ones.
	catIndex := make(map[string]int)
	for i, cat := range Categories {
		// Map existing category names to server categories.
		switch cat.Name {
		case "Essential":
			catIndex["essential"] = i
		case "Development", "Git & GitHub", "DevOps", "Database":
			catIndex["development"] = i
		case "Productivity", "Browsers":
			catIndex["productivity"] = i
		case "NPM Global":
			catIndex["development"] = i // npm goes with development
		}
	}

	for serverCat, newPkgs := range byCat {
		if idx, ok := catIndex[serverCat]; ok {
			Categories[idx].Packages = append(Categories[idx].Packages, newPkgs...)
		} else {
			info := categoryMap[serverCat]
			if info.Name == "" {
				info = struct {
					Name string
					Icon string
				}{Name: serverCat, Icon: "📦"}
			}
			Categories = append(Categories, Category{
				Name:     info.Name,
				Icon:     info.Icon,
				Packages: newPkgs,
			})
		}
	}
}

func updateDescription(name, desc string) {
	if desc == "" {
		return
	}
	for i := range Categories {
		for j := range Categories[i].Packages {
			if Categories[i].Packages[j].Name == name && Categories[i].Packages[j].Description == "" {
				Categories[i].Packages[j].Description = desc
			}
		}
	}
}

func updateInstallerFlags(name, installer string) {
	for i := range Categories {
		for j := range Categories[i].Packages {
			if Categories[i].Packages[j].Name == name {
				switch installer {
				case "cask":
					Categories[i].Packages[j].IsCask = true
				case "npm":
					Categories[i].Packages[j].IsNpm = true
				}
			}
		}
	}
}
