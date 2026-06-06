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

// CaptureResults holds the typed output of each capture step. Fields are
// populated one at a time by CaptureWithProgress and then read by
// assembleSnapshot — no type assertions needed.
type CaptureResults struct {
	Formulae []string
	Casks    []string
	Taps     []string
	Npm      []string
	Bun      []string
	Prefs    []MacOSPref
	Git      *GitSnapshot
	Dotfiles *DotfilesSnapshot
	DevTools []DevTool
	Shell    *ShellSnapshot
}

type captureStep struct {
	name    string
	capture func(r *CaptureResults) error
	count   func(r *CaptureResults) int
}

var captureSteps = []captureStep{
	{"Homebrew Formulae", func(r *CaptureResults) error {
		v, err := CaptureFormulae()
		r.Formulae = v
		return err
	}, func(r *CaptureResults) int { return len(r.Formulae) }},
	{"Homebrew Casks", func(r *CaptureResults) error {
		v, err := CaptureCasks()
		r.Casks = v
		return err
	}, func(r *CaptureResults) int { return len(r.Casks) }},
	{"Homebrew Taps", func(r *CaptureResults) error {
		v, err := CaptureTaps()
		r.Taps = v
		return err
	}, func(r *CaptureResults) int { return len(r.Taps) }},
	{"NPM Global Packages", func(r *CaptureResults) error {
		v, err := CaptureNpm()
		r.Npm = v
		return err
	}, func(r *CaptureResults) int { return len(r.Npm) }},
	{"Bun Global Packages", func(r *CaptureResults) error {
		v, err := CaptureBun()
		r.Bun = v
		return err
	}, func(r *CaptureResults) int { return len(r.Bun) }},
	{"macOS Preferences", func(r *CaptureResults) error {
		v, err := CaptureMacOSPrefs()
		r.Prefs = v
		return err
	}, func(r *CaptureResults) int { return len(r.Prefs) }},
	{"Git Configuration", func(r *CaptureResults) error {
		v, err := CaptureGit()
		r.Git = v
		return err
	}, func(r *CaptureResults) int { return 1 }},
	{"Dotfiles", func(r *CaptureResults) error {
		v, err := CaptureDotfiles()
		r.Dotfiles = v
		return err
	}, func(r *CaptureResults) int {
		if r.Dotfiles != nil && r.Dotfiles.RepoURL != "" {
			return 1
		}
		return 0
	}},
	{"Dev Tools", func(r *CaptureResults) error {
		v, err := CaptureDevTools()
		r.DevTools = v
		return err
	}, func(r *CaptureResults) int { return len(r.DevTools) }},
	{"Shell Config", func(r *CaptureResults) error {
		v, err := CaptureShell()
		r.Shell = v
		return err
	}, func(r *CaptureResults) int {
		if r.Shell != nil && (r.Shell.Theme != "" || len(r.Shell.Plugins) > 0 || r.Shell.OhMyZsh) {
			return 1
		}
		return 0
	}},
}

func assembleSnapshot(r *CaptureResults, failedSteps []string, hostname string) *Snapshot {
	if r.Formulae == nil {
		r.Formulae = []string{}
	}
	if r.Casks == nil {
		r.Casks = []string{}
	}
	if r.Taps == nil {
		r.Taps = []string{}
	}
	if r.Npm == nil {
		r.Npm = []string{}
	}
	if r.Bun == nil {
		r.Bun = []string{}
	}
	if r.Prefs == nil {
		r.Prefs = []MacOSPref{}
	}
	if r.Git == nil {
		r.Git = &GitSnapshot{}
	}
	if r.Dotfiles == nil {
		r.Dotfiles = &DotfilesSnapshot{}
	}
	if r.DevTools == nil {
		r.DevTools = []DevTool{}
	}
	if r.Shell == nil {
		r.Shell = &ShellSnapshot{}
	}

	return &Snapshot{
		Version:    1,
		CapturedAt: time.Now(),
		Hostname:   hostname,
		Packages: PackageSnapshot{
			Formulae: r.Formulae,
			Casks:    r.Casks,
			Taps:     r.Taps,
			Npm:      r.Npm,
			Bun:      r.Bun,
		},
		MacOSPrefs:    r.Prefs,
		Shell:         *r.Shell,
		Git:           *r.Git,
		Dotfiles:      *r.Dotfiles,
		DevTools:      r.DevTools,
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

	results := &CaptureResults{}
	var failedSteps []string
	for i, step := range captureSteps {
		if callback != nil {
			callback(ScanStep{Name: step.name, Index: i, Total: len(captureSteps), Status: "scanning", Count: 0})
		}

		err := step.capture(results)

		if err != nil {
			failedSteps = append(failedSteps, step.name)
			if callback != nil {
				callback(ScanStep{Name: step.name, Index: i, Total: len(captureSteps), Status: "error", Count: 0})
			}
		} else if callback != nil {
			callback(ScanStep{Name: step.name, Index: i, Total: len(captureSteps), Status: "done", Count: step.count(results)})
		}
	}

	return assembleSnapshot(results, failedSteps, hostname), nil
}

// bunListEntryRe matches a single `bun pm ls -g` entry line, after tree-drawing
// characters are stripped. Format: `<name>@<version>` where version starts with
// a digit or `v`. Naming this way (rather than a more permissive pattern) keeps
// the path header line out of the result.
var bunListEntryRe = regexp.MustCompile(`^([@a-zA-Z0-9._/-]+)@[0-9vV]`)

func parseBunList(output string) []string {
	packages := []string{}
	seen := map[string]bool{}
	for _, line := range strings.Split(output, "\n") {
		// Strip tree-drawing characters (├ └ │ ─) and surrounding whitespace.
		line = strings.TrimSpace(line)
		line = strings.TrimLeft(line, "├└│─ \t")
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		m := bunListEntryRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		name := m[1]
		if name == "" || name == "bun" || seen[name] {
			continue
		}
		seen[name] = true
		packages = append(packages, name)
	}
	return packages
}

func CaptureBun() ([]string, error) {
	if _, err := exec.LookPath("bun"); err != nil {
		return []string{}, nil
	}

	output, err := system.RunCommandOutput("bun", "pm", "ls", "-g")
	if err != nil {
		return []string{}, nil
	}

	return parseBunList(output), nil
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
		args := []string{}
		if p.Host == "currentHost" {
			args = append(args, "-currentHost")
		}
		args = append(args, "read", p.Domain, p.Key)

		output, err := system.RunCommandOutput("defaults", args...)
		if err != nil {
			// Key isn't set on this machine — record it with the catalog's
			// default value and the Unset marker so consumers (UI, restore,
			// diff, publish) can distinguish "user has the macOS default"
			// from "user explicitly chose the catalog value".
			prefs = append(prefs, MacOSPref{
				Domain: p.Domain,
				Key:    p.Key,
				Type:   p.Type,
				Value:  p.Value,
				Desc:   p.Desc,
				Host:   p.Host,
				Unset:  true,
			})
			continue
		}

		prefs = append(prefs, MacOSPref{
			Domain: p.Domain,
			Key:    p.Key,
			Type:   p.Type,
			Value:  output,
			Desc:   p.Desc,
			Host:   p.Host,
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
