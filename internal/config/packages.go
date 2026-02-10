package config

import (
	"embed"
	"log"

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

var Categories []Category

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

func GetAllPackageNames() []string {
	var names []string
	for _, cat := range Categories {
		for _, pkg := range cat.Packages {
			names = append(names, pkg.Name)
		}
	}
	return names
}

func IsNpmPackage(name string) bool {
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
	for _, cat := range Categories {
		for _, pkg := range cat.Packages {
			if pkg.Name == name {
				return pkg.IsCask
			}
		}
	}
	return false
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
