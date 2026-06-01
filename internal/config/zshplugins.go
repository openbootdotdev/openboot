package config

import (
	"embed"
	"log"

	"gopkg.in/yaml.v3"
)

//go:embed data/zsh-plugins.yaml
var zshPluginsYAML embed.FS

// ZshPluginRepoURL returns the git repo URL for a known external oh-my-zsh
// plugin name, and whether the name is a known external plugin at all.
//
// The lookup is deliberately conservative: only names with an explicit curated
// URL are treated as external. Built-in OMZ plugins (git, docker, kubectl, ...)
// and unknown/typo'd names are not in the catalog and return ("", false) —
// callers leave those untouched in plugins=() and never attempt a clone.
func ZshPluginRepoURL(name string) (string, bool) {
	for _, p := range loadZshPlugins() {
		if p.Name == name {
			return p.Repo, true
		}
	}
	return "", false
}

func loadZshPlugins() []zshPluginEntry {
	data, err := zshPluginsYAML.ReadFile("data/zsh-plugins.yaml")
	if err != nil {
		log.Printf("Warning: failed to read zsh-plugins.yaml: %v", err)
		return nil
	}

	var zpd zshPluginsData
	if err := yaml.Unmarshal(data, &zpd); err != nil {
		log.Printf("Warning: failed to parse zsh-plugins.yaml: %v", err)
		return nil
	}

	return zpd.Plugins
}
