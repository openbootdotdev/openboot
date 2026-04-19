package snapshot

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/openbootdotdev/openboot/internal/macos"
	"github.com/openbootdotdev/openboot/internal/system"
)

// Capture collects a best-effort snapshot of the current environment.
// Individual step failures are recorded in Snapshot.Health rather than
// aborting the whole capture.
func Capture() (*Snapshot, error) {
	return CaptureWithProgress(nil)
}

type ScanStep struct {
	Name   string `json:"name"`
	Index  int    `json:"index"`
	Total  int    `json:"total"`
	Status string `json:"status"` // "scanning" | "done" | "error"
	Count  int    `json:"count"`
}

type captureStep struct {
	name    string
	capture func() (interface{}, error)
	count   func(interface{}) int
}

func stringsCount(v interface{}) int {
	if s, ok := v.([]string); ok {
		return len(s)
	}
	return 0
}

var captureSteps = []captureStep{
	{"Homebrew Formulae", func() (interface{}, error) { return CaptureFormulae() }, stringsCount},
	{"Homebrew Casks", func() (interface{}, error) { return CaptureCasks() }, stringsCount},
	{"Homebrew Taps", func() (interface{}, error) { return CaptureTaps() }, stringsCount},
	{"NPM Global Packages", func() (interface{}, error) { return CaptureNpm() }, stringsCount},
	{"macOS Preferences", func() (interface{}, error) { return CaptureMacOSPrefs() }, func(v interface{}) int {
		if s, ok := v.([]MacOSPref); ok {
			return len(s)
		}
		return 0
	}},
	{"Git Configuration", func() (interface{}, error) { return CaptureGit() }, func(v interface{}) int { return 1 }},
	{"Dotfiles", func() (interface{}, error) { return CaptureDotfiles() }, func(v interface{}) int {
		if s, ok := v.(*DotfilesSnapshot); ok && s.RepoURL != "" {
			return 1
		}
		return 0
	}},
	{"Dev Tools", func() (interface{}, error) { return CaptureDevTools() }, func(v interface{}) int {
		if s, ok := v.([]DevTool); ok {
			return len(s)
		}
		return 0
	}},
	{"Shell Config", func() (interface{}, error) { return CaptureShell() }, func(v interface{}) int {
		if s, ok := v.(*ShellSnapshot); ok && (s.Theme != "" || len(s.Plugins) > 0 || s.OhMyZsh) {
			return 1
		}
		return 0
	}},
}

func assembleSnapshot(results []interface{}, failedSteps []string, hostname string) *Snapshot {
	formulae, _ := results[0].([]string)
	casks, _ := results[1].([]string)
	taps, _ := results[2].([]string)
	npmPkgs, _ := results[3].([]string)
	prefs, _ := results[4].([]MacOSPref)
	gitSnap, _ := results[5].(*GitSnapshot)
	dotfilesSnap, _ := results[6].(*DotfilesSnapshot)
	devTools, _ := results[7].([]DevTool)
	shellSnap, _ := results[8].(*ShellSnapshot)

	if formulae == nil {
		formulae = []string{}
	}
	if casks == nil {
		casks = []string{}
	}
	if taps == nil {
		taps = []string{}
	}
	if npmPkgs == nil {
		npmPkgs = []string{}
	}
	if prefs == nil {
		prefs = []MacOSPref{}
	}
	if gitSnap == nil {
		gitSnap = &GitSnapshot{}
	}
	if dotfilesSnap == nil {
		dotfilesSnap = &DotfilesSnapshot{}
	}
	if devTools == nil {
		devTools = []DevTool{}
	}
	if shellSnap == nil {
		shellSnap = &ShellSnapshot{}
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
		Dotfiles:      *dotfilesSnap,
		DevTools:      devTools,
		MatchedPreset: "",
		CatalogMatch: CatalogMatch{
			Matched:   []string{},
			Unmatched: []string{},
			MatchRate: 0,
		},
		Health: CaptureHealth{
			FailedSteps: failedSteps,
			Partial:     len(failedSteps) > 0,
		},
	}
}

func CaptureWithProgress(callback func(step ScanStep)) (*Snapshot, error) {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	results := make([]interface{}, len(captureSteps))
	var failedSteps []string
	for i, step := range captureSteps {
		if callback != nil {
			callback(ScanStep{Name: step.name, Index: i, Total: len(captureSteps), Status: "scanning", Count: 0})
		}

		result, err := step.capture()
		results[i] = result

		if err != nil {
			failedSteps = append(failedSteps, step.name)
			if callback != nil {
				callback(ScanStep{Name: step.name, Index: i, Total: len(captureSteps), Status: "error", Count: 0})
			}
		} else if callback != nil {
			callback(ScanStep{Name: step.name, Index: i, Total: len(captureSteps), Status: "done", Count: step.count(result)})
		}
	}

	return assembleSnapshot(results, failedSteps, hostname), nil
}

func CaptureNpm() ([]string, error) {
	if _, err := exec.LookPath("npm"); err != nil {
		return []string{}, nil
	}

	output, err := system.RunCommandOutput("npm", "list", "-g", "--depth=0", "--parseable")
	if err != nil {
		return []string{}, nil
	}

	lines := strings.Split(output, "\n")
	if len(lines) <= 1 {
		return []string{}, nil
	}
	packages := []string{}
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

func captureBrewList(args ...string) ([]string, error) {
	if !isBrewInstalled() {
		return []string{}, nil
	}

	output, err := system.RunCommandOutput("brew", args...)
	if err != nil {
		return []string{}, fmt.Errorf("brew %s: %w", args[0], err)
	}

	return parseLines(output), nil
}

func CaptureFormulae() ([]string, error) {
	return captureBrewList("leaves")
}

func CaptureCasks() ([]string, error) {
	return captureBrewList("list", "--cask")
}

func CaptureTaps() ([]string, error) {
	return captureBrewList("tap")
}

func CaptureMacOSPrefs() ([]MacOSPref, error) {
	prefs := []MacOSPref{}

	for _, p := range macos.DefaultPreferences {
		output, err := system.RunCommandOutput("defaults", "read", p.Domain, p.Key)
		if err != nil {
			continue
		}

		prefs = append(prefs, MacOSPref{
			Domain: p.Domain,
			Key:    p.Key,
			Type:   p.Type,
			Value:  output,
			Desc:   p.Desc,
		})
	}

	return prefs, nil
}

func CaptureGit() (*GitSnapshot, error) {
	snap := &GitSnapshot{}

	if out, err := system.RunCommandOutput("git", "config", "--global", "user.name"); err == nil {
		snap.UserName = out
	}

	if out, err := system.RunCommandOutput("git", "config", "--global", "user.email"); err == nil {
		snap.UserEmail = out
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

func CaptureDevTools() ([]DevTool, error) {
	tools := []DevTool{}

	for _, dt := range devToolCommands {
		if _, err := exec.LookPath(dt.name); err != nil {
			continue
		}

		output, err := system.RunCommandOutput(dt.name, dt.args...)
		if err != nil {
			continue
		}

		version := parseVersion(dt.name, output)
		tools = append(tools, DevTool{
			Name:    dt.name,
			Version: version,
		})
	}

	return tools, nil
}

func CaptureDotfiles() (*DotfilesSnapshot, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return &DotfilesSnapshot{}, nil
	}

	dotfilesPath := filepath.Join(home, ".dotfiles")
	if _, err := os.Stat(filepath.Join(dotfilesPath, ".git")); err != nil {
		return &DotfilesSnapshot{}, nil
	}

	out, err := system.RunCommandOutput("git", "-C", dotfilesPath, "remote", "get-url", "origin")
	if err != nil {
		return &DotfilesSnapshot{}, nil
	}

	return &DotfilesSnapshot{
		RepoURL: out,
	}, nil
}

var (
	zshThemeRe   = regexp.MustCompile(`(?m)^ZSH_THEME="([^"]*)"`)
	zshPluginsRe = regexp.MustCompile(`(?m)^plugins=\((?s:(.*?))\)`)
)

// CaptureShell reads the current Oh-My-Zsh state and .zshrc theme/plugins.
// Returns a zero-value ShellSnapshot (not an error) when .zshrc is absent.
func CaptureShell() (*ShellSnapshot, error) {
	snap := &ShellSnapshot{}

	home, err := os.UserHomeDir()
	if err != nil {
		return snap, nil
	}

	if _, err := os.Stat(filepath.Join(home, ".oh-my-zsh")); err == nil {
		snap.OhMyZsh = true
	}

	raw, err := os.ReadFile(filepath.Join(home, ".zshrc"))
	if err != nil {
		return snap, nil // no .zshrc is fine
	}
	content := string(raw)

	if m := zshThemeRe.FindStringSubmatch(content); len(m) > 1 {
		snap.Theme = m[1]
	}

	if m := zshPluginsRe.FindStringSubmatch(content); len(m) > 1 {
		snap.Plugins = strings.Fields(m[1])
	}

	return snap, nil
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
	output = strings.TrimSpace(output)
	if output == "" {
		return ""
	}

	switch toolName {
	case "go":
		// "go version go1.22.0 darwin/arm64" -> "1.22.0"
		if strings.HasPrefix(output, "go version go") {
			parts := strings.Fields(output)
			if len(parts) >= 3 {
				return strings.TrimPrefix(parts[2], "go")
			}
		}
		return ""
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
		return ""
	case "java":
		lines := strings.Split(output, "\n")
		if len(lines) == 0 {
			return ""
		}
		firstLine := lines[0]
		parts := strings.Fields(firstLine)
		if len(parts) >= 2 {
			return parts[1]
		}
		return ""
	case "ruby":
		// "ruby 3.2.2 (2023-03-30 revision e51014f9c0) ..." -> "3.2.2"
		parts := strings.Fields(output)
		if len(parts) >= 2 {
			return parts[1]
		}
		return ""
	case "docker":
		// "Docker version 24.0.7, build afdd53b" -> "24.0.7"
		parts := strings.Fields(output)
		if len(parts) >= 3 {
			return strings.TrimSuffix(parts[2], ",")
		}
		return ""
	}
	return output
}
