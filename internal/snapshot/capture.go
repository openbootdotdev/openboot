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
)

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

func CaptureWithProgress(callback func(step ScanStep)) (*Snapshot, error) {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	steps := []captureStep{
		{"Homebrew Formulae", func() (interface{}, error) { return CaptureFormulae() }, func(v interface{}) int { return len(v.([]string)) }},
		{"Homebrew Casks", func() (interface{}, error) { return CaptureCasks() }, func(v interface{}) int { return len(v.([]string)) }},
		{"Homebrew Taps", func() (interface{}, error) { return CaptureTaps() }, func(v interface{}) int { return len(v.([]string)) }},
		{"NPM Global Packages", func() (interface{}, error) { return CaptureNpm() }, func(v interface{}) int { return len(v.([]string)) }},
		{"macOS Preferences", func() (interface{}, error) { return CaptureMacOSPrefs() }, func(v interface{}) int { return len(v.([]MacOSPref)) }},
		{"Shell Environment", func() (interface{}, error) { return CaptureShell() }, func(v interface{}) int { return 1 }},
		{"Git Configuration", func() (interface{}, error) { return CaptureGit() }, func(v interface{}) int { return 1 }},
		{"Dev Tools", func() (interface{}, error) { return CaptureDevTools() }, func(v interface{}) int { return len(v.([]DevTool)) }},
	}

	results := make([]interface{}, len(steps))
	var failedSteps []string
	for i, step := range steps {
		if callback != nil {
			callback(ScanStep{Name: step.name, Index: i, Total: len(steps), Status: "scanning", Count: 0})
		}

		result, err := step.capture()
		results[i] = result

		if err != nil {
			failedSteps = append(failedSteps, step.name)
			if callback != nil {
				callback(ScanStep{Name: step.name, Index: i, Total: len(steps), Status: "error", Count: 0})
			}
		} else if callback != nil {
			callback(ScanStep{Name: step.name, Index: i, Total: len(steps), Status: "done", Count: step.count(result)})
		}
	}

	formulae := results[0].([]string)
	casks := results[1].([]string)
	taps := results[2].([]string)
	npmPkgs := results[3].([]string)
	prefs := results[4].([]MacOSPref)
	shellSnap := results[5].(*ShellSnapshot)
	gitSnap := results[6].(*GitSnapshot)
	devTools := results[7].([]DevTool)

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
		Health: CaptureHealth{
			FailedSteps: failedSteps,
			Partial:     len(failedSteps) > 0,
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
	if len(lines) <= 1 {
		return []string{}, nil
	}
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

func captureBrewList(args ...string) ([]string, error) {
	if !isBrewInstalled() {
		return []string{}, nil
	}

	cmd := exec.Command("brew", args...)
	output, err := cmd.Output()
	if err != nil {
		return []string{}, fmt.Errorf("brew %s: %w", args[0], err)
	}

	return parseLines(string(output)), nil
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
