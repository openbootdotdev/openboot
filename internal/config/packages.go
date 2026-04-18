package config

import (
	"embed"
	"log"
	"sync"

	"gopkg.in/yaml.v3"
)

//go:embed data/packages.yaml
var packagesYAML embed.FS

type Package struct {
	Name        string `yaml:"name"`
	Description string `yaml:"desc"`
	IsCask      bool   `yaml:"cask"`
	IsNpm       bool   `yaml:"npm"`
}

type Category struct {
	Name     string    `yaml:"name"`
	Icon     string    `yaml:"icon"`
	Packages []Package `yaml:"packages"`
}

type packagesData struct {
	Categories []Category `yaml:"categories"`
}

var (
	Categories   []Category
	categoriesMu sync.RWMutex
)

func init() {
	data, err := packagesYAML.ReadFile("data/packages.yaml")
	if err != nil {
		log.Fatalf("Failed to read packages.yaml: %v", err)
	}

	var pd packagesData
	if err := yaml.Unmarshal(data, &pd); err != nil {
		log.Fatalf("Failed to parse packages.yaml: %v", err)
	}

	Categories = pd.Categories
}

func GetPackagesForPreset(presetName string) map[string]bool {
	selected := make(map[string]bool)

	preset, ok := Presets[presetName]
	if !ok {
		return selected
	}

	for _, pkg := range preset.CLI {
		selected[pkg] = true
	}
	for _, pkg := range preset.Cask {
		selected[pkg] = true
	}
	for _, pkg := range preset.Npm {
		selected[pkg] = true
	}

	return selected
}

// GetCategories returns a deep copy of the package catalog under RLock.
// Callers must not modify the returned slice.
func GetCategories() []Category {
	categoriesMu.RLock()
	defer categoriesMu.RUnlock()
	result := make([]Category, len(Categories))
	for i, cat := range Categories {
		pkgs := make([]Package, len(cat.Packages))
		copy(pkgs, cat.Packages)
		result[i] = cat
		result[i].Packages = pkgs
	}
	return result
}

func GetAllPackageNames() []string {
	categoriesMu.RLock()
	defer categoriesMu.RUnlock()
	var names []string
	for _, cat := range Categories {
		for _, pkg := range cat.Packages {
			names = append(names, pkg.Name)
		}
	}
	return names
}

func IsNpmPackage(name string) bool {
	categoriesMu.RLock()
	defer categoriesMu.RUnlock()
	for _, cat := range Categories {
		for _, pkg := range cat.Packages {
			if pkg.Name == name {
				return pkg.IsNpm
			}
		}
	}
	return false
}

func IsCaskPackage(name string) bool {
	categoriesMu.RLock()
	defer categoriesMu.RUnlock()
	for _, cat := range Categories {
		for _, pkg := range cat.Packages {
			if pkg.Name == name {
				return pkg.IsCask
			}
		}
	}
	return false
}

// CatalogDescriptionMap builds a name → description lookup from the embedded
// packages catalog. Use as a fallback when PackageEntry.Desc is empty.
func CatalogDescriptionMap() map[string]string {
	categoriesMu.RLock()
	defer categoriesMu.RUnlock()
	m := make(map[string]string)
	for _, cat := range Categories {
		for _, pkg := range cat.Packages {
			if pkg.Description != "" {
				m[pkg.Name] = pkg.Description
			}
		}
	}
	return m
}

func IsTapPackage(name string) bool {
	parts := 0
	for _, c := range name {
		if c == '/' {
			parts++
		}
	}
	return parts == 2
}
