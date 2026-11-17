package brew

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/openbootdotdev/openboot/internal/ui"
)

// filterExternalApps preserves app bundles occupying a cask's destination.
// This is conflict avoidance, not identity verification or Homebrew adoption:
// skipped casks must never be returned as successfully installed by Homebrew.
// Use brew's resolved targets so custom app directories and renamed artifacts
// work without guessing from a cask token. Older brew versions without targets
// and complex installers retain the normal brew install/error path.
func filterExternalApps(packages []string) []string {
	if len(packages) == 0 {
		return packages
	}
	args := append([]string{"info", "--cask", "--json=v2"}, packages...)
	data, err := currentRunner().Output(args...)
	if err != nil {
		ui.Warn("Could not check for externally installed apps; continuing with normal Homebrew installation")
		return packages
	}
	var info struct {
		Casks []struct {
			Token     string                       `json:"token"`
			FullToken string                       `json:"full_token"`
			Installed json.RawMessage              `json:"installed"`
			Artifacts []map[string]json.RawMessage `json:"artifacts"`
		} `json:"casks"`
	}
	if err := json.Unmarshal(data, &info); err != nil {
		ui.Warn("Could not read cask app destinations; continuing with normal Homebrew installation")
		return packages
	}
	external := make(map[string][]string)
	for _, cask := range info.Casks {
		// Installed metadata is authoritative even if a preceding list raced
		// with a separate Homebrew installation.
		if len(cask.Installed) > 0 && string(cask.Installed) != "null" {
			continue
		}
		paths := existingAppTargets(cask.Artifacts)
		if len(paths) > 0 {
			external[cask.Token] = paths
			external[cask.FullToken] = paths
		}
	}
	var pending []string
	for _, pkg := range packages {
		if paths := external[pkg]; len(paths) > 0 {
			ui.Warn(fmt.Sprintf("Skipping %s: existing app at %s; keeping it unchanged (not verified or managed by Homebrew). To use Homebrew, resolve the app conflict and rerun.", pkg, strings.Join(paths, ", ")))
			continue
		}
		pending = append(pending, pkg)
	}
	return pending
}

func existingAppTargets(artifacts []map[string]json.RawMessage) []string {
	var paths []string
	for _, artifact := range artifacts {
		for kind := range artifact {
			switch kind {
			case "app", "target", "zap", "uninstall":
			default:
				// Do not skip pkg installers, hooks, binaries or other artifacts
				// just because an app happens to be present.
				return nil
			}
		}
		if _, ok := artifact["app"]; !ok {
			continue
		}
		var target string
		if json.Unmarshal(artifact["target"], &target) != nil || !filepath.IsAbs(target) || filepath.Ext(target) != ".app" {
			return nil
		}
		bundle, err := os.Stat(target)
		if err != nil || !bundle.IsDir() {
			return nil
		}
		plist, err := os.Stat(filepath.Join(target, "Contents", "Info.plist"))
		if err != nil || !plist.Mode().IsRegular() {
			return nil
		}
		paths = append(paths, target)
	}
	return paths
}
