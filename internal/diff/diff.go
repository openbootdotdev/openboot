package diff

import "sort"

// Source describes where the reference configuration came from.
type Source struct {
	Kind string // "file", "local", "remote"
	Path string // file path, "~/.openboot/snapshot.json", or "user/slug"
}

// ListDiff is the result of comparing two string lists (e.g. formulae).
type ListDiff struct {
	Missing []string // in reference but not in system
	Extra   []string // in system but not in reference
	Common  int      // count of items in both
}

// ValueChange records a single scalar value that differs.
type ValueChange struct {
	System    string
	Reference string
}

// PackageDiff holds diffs for all package categories.
type PackageDiff struct {
	Formulae ListDiff
	Casks    ListDiff
	Npm      ListDiff
	Taps     ListDiff
}

// MacOSDiff holds macOS preference differences.
type MacOSDiff struct {
	Changed []MacOSPrefChange
	Missing []MacOSPrefEntry // in reference but not in system
	Extra   []MacOSPrefEntry // in system but not in reference
}

// MacOSPrefChange represents a preference that differs between system and reference.
type MacOSPrefChange struct {
	Domain    string
	Key       string
	System    string
	Reference string
}

// MacOSPrefEntry represents a single macOS preference.
type MacOSPrefEntry struct {
	Domain string
	Key    string
	Value  string
}

// DevToolDiff holds dev tool differences.
type DevToolDiff struct {
	Missing []string      // in reference but not in system (by name)
	Extra   []string      // in system but not in reference
	Changed []DevToolDelta // same name, different version
	Common  int
}

// DevToolDelta records a dev tool version difference.
type DevToolDelta struct {
	Name      string
	System    string
	Reference string
}

// DotfilesDiff holds dotfiles repository differences.
type DotfilesDiff struct {
	RepoChanged *ValueChange // nil = same or unavailable
	Dirty       bool         // uncommitted changes in local repo
	Unpushed    bool         // local commits not pushed to remote
}

// DiffResult is the top-level diff output.
type DiffResult struct {
	Source   Source
	Packages PackageDiff
	MacOS    *MacOSDiff    // nil when not compared
	DevTools *DevToolDiff  // nil when not compared
	Dotfiles *DotfilesDiff // nil when not compared
}

// DiffLists computes a bidirectional set diff between system and reference string slices.
// Both inputs may be nil; duplicates are ignored.
func DiffLists(system, reference []string) ListDiff {
	sysSet := toSet(system)
	refSet := toSet(reference)

	var missing []string
	var extra []string
	common := 0

	for item := range refSet {
		if sysSet[item] {
			common++
		} else {
			missing = append(missing, item)
		}
	}

	for item := range sysSet {
		if !refSet[item] {
			extra = append(extra, item)
		}
	}

	sort.Strings(missing)
	sort.Strings(extra)

	return ListDiff{
		Missing: missing,
		Extra:   extra,
		Common:  common,
	}
}

// HasChanges reports whether the diff result contains any differences.
func (r *DiffResult) HasChanges() bool {
	if r.TotalMissing() > 0 || r.TotalExtra() > 0 || r.TotalChanged() > 0 {
		return true
	}
	if r.Dotfiles != nil && (r.Dotfiles.Dirty || r.Dotfiles.Unpushed || r.Dotfiles.RepoChanged != nil) {
		return true
	}
	return false
}

// TotalMissing returns the count of items in the reference but missing from the system.
func (r *DiffResult) TotalMissing() int {
	n := len(r.Packages.Formulae.Missing) +
		len(r.Packages.Casks.Missing) +
		len(r.Packages.Npm.Missing) +
		len(r.Packages.Taps.Missing)

	if r.MacOS != nil {
		n += len(r.MacOS.Missing)
	}
	if r.DevTools != nil {
		n += len(r.DevTools.Missing)
	}
	return n
}

// TotalExtra returns the count of items in the system but not in the reference.
func (r *DiffResult) TotalExtra() int {
	n := len(r.Packages.Formulae.Extra) +
		len(r.Packages.Casks.Extra) +
		len(r.Packages.Npm.Extra) +
		len(r.Packages.Taps.Extra)

	if r.MacOS != nil {
		n += len(r.MacOS.Extra)
	}
	if r.DevTools != nil {
		n += len(r.DevTools.Extra)
	}
	return n
}

// TotalChanged returns the count of values that differ between system and reference.
func (r *DiffResult) TotalChanged() int {
	n := 0
	if r.MacOS != nil {
		n += len(r.MacOS.Changed)
	}
	if r.DevTools != nil {
		n += len(r.DevTools.Changed)
	}
	if r.Dotfiles != nil && r.Dotfiles.RepoChanged != nil {
		n++
	}
	return n
}

func toSet(items []string) map[string]bool {
	s := make(map[string]bool, len(items))
	for _, item := range items {
		s[item] = true
	}
	return s
}
