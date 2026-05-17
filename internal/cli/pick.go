package cli

import (
	"strings"

	"github.com/openbootdotdev/openboot/internal/config"
)

// ParsePicks splits a comma-separated --pick value into a set.
// Whitespace around names is trimmed; empty entries are skipped.
func ParsePicks(raw string) map[string]bool {
	out := map[string]bool{}
	if raw == "" {
		return out
	}
	for _, part := range strings.Split(raw, ",") {
		name := strings.TrimSpace(part)
		if name == "" {
			continue
		}
		out[name] = true
	}
	return out
}

// ApplyPicks returns a copy of rc whose Packages, Casks, and Npm
// slices contain only entries whose Name appears in picks. Taps,
// dotfiles, shell, macOS prefs, post-install, and other fields are
// passed through unchanged. Any names in picks that didn't match any
// package are returned in unknown so the caller can fail fast (--pick)
// or ignore (TUI, where picks come from rc itself).
func ApplyPicks(rc *config.RemoteConfig, picks map[string]bool) (filtered *config.RemoteConfig, unknown []string) {
	cp := *rc
	cp.Packages = filterEntries(rc.Packages, picks)
	cp.Casks = filterEntries(rc.Casks, picks)
	cp.Npm = filterEntries(rc.Npm, picks)

	matched := map[string]bool{}
	for _, e := range cp.Packages {
		matched[e.Name] = true
	}
	for _, e := range cp.Casks {
		matched[e.Name] = true
	}
	for _, e := range cp.Npm {
		matched[e.Name] = true
	}

	for name := range picks {
		if !matched[name] {
			unknown = append(unknown, name)
		}
	}
	return &cp, unknown
}

func filterEntries(in config.PackageEntryList, picks map[string]bool) config.PackageEntryList {
	out := make(config.PackageEntryList, 0, len(in))
	for _, e := range in {
		if picks[e.Name] {
			out = append(out, e)
		}
	}
	return out
}
