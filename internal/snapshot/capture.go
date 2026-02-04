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
