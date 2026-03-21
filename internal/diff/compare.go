package diff

import (
	"fmt"
	"sort"

	"github.com/openbootdotdev/openboot/internal/config"
	"github.com/openbootdotdev/openboot/internal/snapshot"
)

// CompareSnapshots performs a full diff between the current system snapshot and a reference snapshot.
// All sections (packages, shell, git, macOS, dev tools) are compared.
func CompareSnapshots(system, reference *snapshot.Snapshot, source Source) *DiffResult {
	result := &DiffResult{
		Source:   source,
		Packages: diffPackages(system, reference),
		Shell:    diffShell(system.Shell, reference.Shell),
		Git:      diffGit(system.Git, reference.Git),
		MacOS:    diffMacOS(system.MacOSPrefs, reference.MacOSPrefs),
		DevTools: diffDevTools(system.DevTools, reference.DevTools),
	}
	return result
}

// CompareSnapshotToRemote compares the system snapshot against a remote config.
// Remote configs only contain package data, so Shell/Git/MacOS/DevTools are nil.
func CompareSnapshotToRemote(system *snapshot.Snapshot, remote *config.RemoteConfig, source Source) *DiffResult {
	return &DiffResult{
		Source: source,
		Packages: PackageDiff{
			Formulae: DiffLists(system.Packages.Formulae, remote.Packages.Names()),
			Casks:    DiffLists(system.Packages.Casks, remote.Casks.Names()),
			Npm:      DiffLists(system.Packages.Npm, remote.Npm.Names()),
			Taps:     DiffLists(system.Packages.Taps, remote.Taps),
		},
	}
}

func diffPackages(system, reference *snapshot.Snapshot) PackageDiff {
	return PackageDiff{
		Formulae: DiffLists(system.Packages.Formulae, reference.Packages.Formulae),
		Casks:    DiffLists(system.Packages.Casks, reference.Packages.Casks),
		Npm:      DiffLists(system.Packages.Npm, reference.Packages.Npm),
		Taps:     DiffLists(system.Packages.Taps, reference.Packages.Taps),
	}
}

func diffShell(system, reference snapshot.ShellSnapshot) *ShellDiff {
	sd := &ShellDiff{
		Plugins: DiffLists(system.Plugins, reference.Plugins),
	}

	if system.Theme != reference.Theme {
		sd.Theme = &ValueChange{System: system.Theme, Reference: reference.Theme}
	}

	sysOMZ := fmt.Sprintf("%t", system.OhMyZsh)
	refOMZ := fmt.Sprintf("%t", reference.OhMyZsh)
	if sysOMZ != refOMZ {
		sd.OhMyZsh = &ValueChange{System: sysOMZ, Reference: refOMZ}
	}

	return sd
}

func diffGit(system, reference snapshot.GitSnapshot) *GitDiff {
	gd := &GitDiff{}

	if system.UserName != reference.UserName {
		gd.UserName = &ValueChange{System: system.UserName, Reference: reference.UserName}
	}
	if system.UserEmail != reference.UserEmail {
		gd.UserEmail = &ValueChange{System: system.UserEmail, Reference: reference.UserEmail}
	}

	if gd.UserName == nil && gd.UserEmail == nil {
		return nil
	}
	return gd
}

func diffMacOS(system, reference []snapshot.MacOSPref) *MacOSDiff {
	type prefKey struct {
		Domain string
		Key    string
	}

	sysMap := make(map[prefKey]string, len(system))
	for _, p := range system {
		sysMap[prefKey{p.Domain, p.Key}] = p.Value
	}

	refMap := make(map[prefKey]string, len(reference))
	for _, p := range reference {
		refMap[prefKey{p.Domain, p.Key}] = p.Value
	}

	md := &MacOSDiff{}

	// Find missing and changed
	for _, p := range reference {
		pk := prefKey{p.Domain, p.Key}
		sysVal, exists := sysMap[pk]
		if !exists {
			md.Missing = append(md.Missing, MacOSPrefEntry{
				Domain: p.Domain, Key: p.Key, Value: p.Value,
			})
		} else if sysVal != p.Value {
			md.Changed = append(md.Changed, MacOSPrefChange{
				Domain: p.Domain, Key: p.Key, System: sysVal, Reference: p.Value,
			})
		}
	}

	// Find extra (in system but not in reference)
	for _, p := range system {
		pk := prefKey{p.Domain, p.Key}
		if _, exists := refMap[pk]; !exists {
			md.Extra = append(md.Extra, MacOSPrefEntry{
				Domain: p.Domain, Key: p.Key, Value: p.Value,
			})
		}
	}

	return md
}

func diffDevTools(system, reference []snapshot.DevTool) *DevToolDiff {
	sysMap := make(map[string]string, len(system))
	for _, t := range system {
		sysMap[t.Name] = t.Version
	}

	refMap := make(map[string]string, len(reference))
	for _, t := range reference {
		refMap[t.Name] = t.Version
	}

	dd := &DevToolDiff{}

	// Find missing and changed
	for _, t := range reference {
		sysVer, exists := sysMap[t.Name]
		if !exists {
			dd.Missing = append(dd.Missing, t.Name)
		} else if sysVer != t.Version {
			dd.Changed = append(dd.Changed, DevToolDelta{
				Name: t.Name, System: sysVer, Reference: t.Version,
			})
		} else {
			dd.Common++
		}
	}

	// Find extra
	for _, t := range system {
		if _, exists := refMap[t.Name]; !exists {
			dd.Extra = append(dd.Extra, t.Name)
		}
	}

	sort.Strings(dd.Missing)
	sort.Strings(dd.Extra)

	return dd
}
