package snapshot

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/openbootdotdev/openboot/internal/macos"
)

// Capture orchestrates a full environment snapshot.
func Capture() (*Snapshot, error) {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	formulae, err := CaptureFormulae()
	if err != nil {
		return nil, err
	}

	casks, err := CaptureCasks()
	if err != nil {
		return nil, err
	}

	taps, err := CaptureTaps()
	if err != nil {
		return nil, err
	}

	npmPkgs, err := CaptureNpm()
	if err != nil {
		return nil, err
	}

	prefs, err := CaptureMacOSPrefs()
	if err != nil {
		return nil, err
	}

	shellSnap, err := CaptureShell()
	if err != nil {
		return nil, err
	}

	gitSnap, err := CaptureGit()
	if err != nil {
		return nil, err
	}

	devTools, err := CaptureDevTools()
	if err != nil {
		return nil, err
	}

	return &Snapshot{
		Version:    1,
		CapturedAt: time.Now(),
		Hostname:   hostname,
		Packages: PackageSnapshot{
			Formulae: formulae,
			Casks:    casks,
			Taps:     taps,
			Npm:      npmPkgs,
		},
		MacOSPrefs:    prefs,
		Shell:         *shellSnap,
		Git:           *gitSnap,
		DevTools:      devTools,
		MatchedPreset: "",
		CatalogMatch: CatalogMatch{
			Matched:   []string{},
			Unmatched: []string{},
			MatchRate: 0,
		},
	}, nil
}

// ScanStep represents progress information for a single capture step.
type ScanStep struct {
	Name   string `json:"name"`   // e.g. "Homebrew Formulae"
	Index  int    `json:"index"`  // 0-7
	Total  int    `json:"total"`  // always 8
	Status string `json:"status"` // "scanning" | "done" | "error"
	Count  int    `json:"count"`  // items found (only meaningful on "done")
}

// CaptureWithProgress orchestrates a full environment snapshot with progress callbacks.
// The callback is invoked before and after each capture step.
// If callback is nil, it is not invoked.
func CaptureWithProgress(callback func(step ScanStep)) (*Snapshot, error) {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	// Step 0: Homebrew Formulae
	if callback != nil {
		callback(ScanStep{Name: "Homebrew Formulae", Index: 0, Total: 8, Status: "scanning", Count: 0})
	}
	formulae, err := CaptureFormulae()
	if err != nil {
		if callback != nil {
			callback(ScanStep{Name: "Homebrew Formulae", Index: 0, Total: 8, Status: "error", Count: 0})
		}
	} else {
		if callback != nil {
			callback(ScanStep{Name: "Homebrew Formulae", Index: 0, Total: 8, Status: "done", Count: len(formulae)})
		}
	}

	// Step 1: Homebrew Casks
	if callback != nil {
		callback(ScanStep{Name: "Homebrew Casks", Index: 1, Total: 8, Status: "scanning", Count: 0})
	}
	casks, err := CaptureCasks()
	if err != nil {
		if callback != nil {
			callback(ScanStep{Name: "Homebrew Casks", Index: 1, Total: 8, Status: "error", Count: 0})
		}
	} else {
		if callback != nil {
			callback(ScanStep{Name: "Homebrew Casks", Index: 1, Total: 8, Status: "done", Count: len(casks)})
		}
	}

	// Step 2: Homebrew Taps
	if callback != nil {
		callback(ScanStep{Name: "Homebrew Taps", Index: 2, Total: 8, Status: "scanning", Count: 0})
	}
	taps, err := CaptureTaps()
	if err != nil {
		if callback != nil {
			callback(ScanStep{Name: "Homebrew Taps", Index: 2, Total: 8, Status: "error", Count: 0})
		}
	} else {
		if callback != nil {
			callback(ScanStep{Name: "Homebrew Taps", Index: 2, Total: 8, Status: "done", Count: len(taps)})
		}
	}

	// Step 3: NPM Global Packages
	if callback != nil {
		callback(ScanStep{Name: "NPM Global Packages", Index: 3, Total: 8, Status: "scanning", Count: 0})
	}
	npmPkgs, err := CaptureNpm()
	if err != nil {
		if callback != nil {
			callback(ScanStep{Name: "NPM Global Packages", Index: 3, Total: 8, Status: "error", Count: 0})
		}
	} else {
		if callback != nil {
			callback(ScanStep{Name: "NPM Global Packages", Index: 3, Total: 8, Status: "done", Count: len(npmPkgs)})
		}
	}

	// Step 4: macOS Preferences
	if callback != nil {
		callback(ScanStep{Name: "macOS Preferences", Index: 4, Total: 8, Status: "scanning", Count: 0})
	}
	prefs, err := CaptureMacOSPrefs()
	if err != nil {
		if callback != nil {
			callback(ScanStep{Name: "macOS Preferences", Index: 4, Total: 8, Status: "error", Count: 0})
		}
	} else {
		if callback != nil {
			callback(ScanStep{Name: "macOS Preferences", Index: 4, Total: 8, Status: "done", Count: len(prefs)})
		}
	}

	// Step 5: Shell Environment
	if callback != nil {
		callback(ScanStep{Name: "Shell Environment", Index: 5, Total: 8, Status: "scanning", Count: 0})
	}
	shellSnap, err := CaptureShell()
	if err != nil {
		if callback != nil {
			callback(ScanStep{Name: "Shell Environment", Index: 5, Total: 8, Status: "error", Count: 0})
		}
	} else {
		if callback != nil {
			callback(ScanStep{Name: "Shell Environment", Index: 5, Total: 8, Status: "done", Count: 1})
		}
	}

	// Step 6: Git Configuration
	if callback != nil {
		callback(ScanStep{Name: "Git Configuration", Index: 6, Total: 8, Status: "scanning", Count: 0})
	}
	gitSnap, err := CaptureGit()
	if err != nil {
		if callback != nil {
			callback(ScanStep{Name: "Git Configuration", Index: 6, Total: 8, Status: "error", Count: 0})
		}
	} else {
		if callback != nil {
			callback(ScanStep{Name: "Git Configuration", Index: 6, Total: 8, Status: "done", Count: 1})
		}
	}

	// Step 7: Dev Tools
	if callback != nil {
		callback(ScanStep{Name: "Dev Tools", Index: 7, Total: 8, Status: "scanning", Count: 0})
	}
	devTools, err := CaptureDevTools()
	if err != nil {
		if callback != nil {
			callback(ScanStep{Name: "Dev Tools", Index: 7, Total: 8, Status: "error", Count: 0})
		}
	} else {
		if callback != nil {
			callback(ScanStep{Name: "Dev Tools", Index: 7, Total: 8, Status: "done", Count: len(devTools)})
		}
	}

	return &Snapshot{
		Version:    1,
		CapturedAt: time.Now(),
		Hostname:   hostname,
		Packages: PackageSnapshot{
			Formulae: formulae,
			Casks:    casks,
			Taps:     taps,
			Npm:      npmPkgs,
		},
		MacOSPrefs:    prefs,
		Shell:         *shellSnap,
		Git:           *gitSnap,
		DevTools:      devTools,
		MatchedPreset: "",
		CatalogMatch: CatalogMatch{
			Matched:   []string{},
			Unmatched: []string{},
			MatchRate: 0,
		},
	}, nil
}

func CaptureNpm() ([]string, error) {
	if _, err := exec.LookPath("npm"); err != nil {
		return []string{}, nil
	}

	cmd := exec.Command("npm", "list", "-g", "--depth=0", "--parseable")
	output, err := cmd.Output()
	if err != nil {
		return []string{}, nil
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var packages []string
	for _, line := range lines[1:] {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "/")
		if len(parts) > 0 {
			pkgName := parts[len(parts)-1]
			if len(parts) >= 2 && strings.HasPrefix(parts[len(parts)-2], "@") {
				pkgName = parts[len(parts)-2] + "/" + parts[len(parts)-1]
			}
			if pkgName != "" && pkgName != "npm" && pkgName != "corepack" {
				packages = append(packages, pkgName)
			}
		}
	}
	return packages, nil
}

func isBrewInstalled() bool {
	_, err := exec.LookPath("brew")
	return err == nil
}

// CaptureFormulae returns user-intentional formulae via `brew leaves`.
func CaptureFormulae() ([]string, error) {
	if !isBrewInstalled() {
		return []string{}, nil
	}

	cmd := exec.Command("brew", "leaves")
	output, err := cmd.Output()
	if err != nil {
		return []string{}, nil
	}

	return parseLines(string(output)), nil
}

// CaptureCasks returns installed casks via `brew list --cask`.
func CaptureCasks() ([]string, error) {
	if !isBrewInstalled() {
		return []string{}, nil
	}

	cmd := exec.Command("brew", "list", "--cask")
	output, err := cmd.Output()
	if err != nil {
		return []string{}, nil
	}

	return parseLines(string(output)), nil
}

// CaptureTaps returns active Homebrew taps.
func CaptureTaps() ([]string, error) {
	if !isBrewInstalled() {
		return []string{}, nil
	}

	cmd := exec.Command("brew", "tap")
	output, err := cmd.Output()
	if err != nil {
		return []string{}, nil
	}

	return parseLines(string(output)), nil
}

// CaptureMacOSPrefs reads the current values of whitelisted macOS preferences.
func CaptureMacOSPrefs() ([]MacOSPref, error) {
	prefs := []MacOSPref{}

	for _, p := range macos.DefaultPreferences {
		cmd := exec.Command("defaults", "read", p.Domain, p.Key)
		output, err := cmd.Output()
		if err != nil {
			continue
		}

		prefs = append(prefs, MacOSPref{
			Domain: p.Domain,
			Key:    p.Key,
			Value:  strings.TrimSpace(string(output)),
			Desc:   p.Desc,
		})
	}

	return prefs, nil
}

// CaptureShell detects the default shell, Oh-My-Zsh, plugins, and theme.
func CaptureShell() (*ShellSnapshot, error) {
	snap := &ShellSnapshot{
		Default: os.Getenv("SHELL"),
		Plugins: []string{},
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return snap, nil
	}

	omzDir := filepath.Join(home, ".oh-my-zsh")
	if _, err := os.Stat(omzDir); err == nil {
		snap.OhMyZsh = true
	}

	zshrc := filepath.Join(home, ".zshrc")
	data, err := os.ReadFile(zshrc)
	if err != nil {
		return snap, nil
	}
	content := string(data)

	pluginsRe := regexp.MustCompile(`plugins=\(([^)]*)\)`)
	if m := pluginsRe.FindStringSubmatch(content); len(m) > 1 {
		for _, p := range strings.Fields(m[1]) {
			p = strings.TrimSpace(p)
			if p != "" {
				snap.Plugins = append(snap.Plugins, p)
			}
		}
	}

	themeRe := regexp.MustCompile(`ZSH_THEME="([^"]*)"`)
	if m := themeRe.FindStringSubmatch(content); len(m) > 1 {
		snap.Theme = m[1]
	}

	return snap, nil
}

// CaptureGit reads global git user.name and user.email.
func CaptureGit() (*GitSnapshot, error) {
	snap := &GitSnapshot{}

	if out, err := exec.Command("git", "config", "--global", "user.name").Output(); err == nil {
		snap.UserName = strings.TrimSpace(string(out))
	}

	if out, err := exec.Command("git", "config", "--global", "user.email").Output(); err == nil {
		snap.UserEmail = strings.TrimSpace(string(out))
	}

	return snap, nil
}

var devToolCommands = []struct {
	name string
	args []string
}{
	{"go", []string{"version"}},
	{"node", []string{"--version"}},
	{"python3", []string{"--version"}},
	{"rustc", []string{"--version"}},
	{"java", []string{"--version"}},
	{"ruby", []string{"--version"}},
	{"docker", []string{"--version"}},
}

// CaptureDevTools detects installed development tools and their versions.
func CaptureDevTools() ([]DevTool, error) {
	tools := []DevTool{}

	for _, dt := range devToolCommands {
		if _, err := exec.LookPath(dt.name); err != nil {
			continue
		}

		cmd := exec.Command(dt.name, dt.args...)
		output, err := cmd.Output()
		if err != nil {
			continue
		}

		version := parseVersion(dt.name, strings.TrimSpace(string(output)))
		tools = append(tools, DevTool{
			Name:    dt.name,
			Version: version,
		})
	}

	return tools, nil
}

func sanitizePath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if strings.HasPrefix(path, home+"/") {
		return "~/" + path[len(home)+1:]
	}
	if path == home {
		return "~"
	}
	return path
}

func parseLines(output string) []string {
	lines := []string{}
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func parseVersion(toolName, output string) string {
	switch toolName {
	case "go":
		// "go version go1.22.0 darwin/arm64" -> "1.22.0"
		if strings.HasPrefix(output, "go version go") {
			parts := strings.Fields(output)
			if len(parts) >= 3 {
				return strings.TrimPrefix(parts[2], "go")
			}
		}
	case "node":
		// "v20.11.0" -> "20.11.0"
		return strings.TrimPrefix(output, "v")
	case "python3":
		// "Python 3.12.0" -> "3.12.0"
		return strings.TrimPrefix(output, "Python ")
	case "rustc":
		// "rustc 1.75.0 (82e1608df 2023-12-21)" -> "1.75.0"
		parts := strings.Fields(output)
		if len(parts) >= 2 {
			return parts[1]
		}
	case "java":
		// First line: 'openjdk 21.0.1 2023-10-17' or 'java 21.0.1 ...'
		firstLine := strings.Split(output, "\n")[0]
		parts := strings.Fields(firstLine)
		if len(parts) >= 2 {
			return parts[1]
		}
	case "ruby":
		// "ruby 3.2.2 (2023-03-30 revision e51014f9c0) ..." -> "3.2.2"
		parts := strings.Fields(output)
		if len(parts) >= 2 {
			return parts[1]
		}
	case "docker":
		// "Docker version 24.0.7, build afdd53b" -> "24.0.7"
		parts := strings.Fields(output)
		if len(parts) >= 3 {
			return strings.TrimSuffix(parts[2], ",")
		}
	}
	return output
}
