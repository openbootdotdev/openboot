package doctor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/openbootdotdev/openboot/internal/system"
	"github.com/openbootdotdev/openboot/internal/ui"
)

// Status represents the outcome of a single diagnostic check.
type Status int

const (
	StatusOK   Status = iota
	StatusWarn        // non-fatal advisory
	StatusFail        // something the user should fix
)

// Result holds the outcome of a single check.
type Result struct {
	Status  Status
	Message string
}

// Summary counts results by status.
type Summary struct {
	Passed   int
	Warnings int
	Errors   int
}

// CommandRunner abstracts subprocess execution for testability.
// Production code wires this to system.RunCommandSilent / system.RunCommandOutput.
type CommandRunner interface {
	// RunSilent runs a command and returns combined stdout+stderr.
	RunSilent(name string, args ...string) (string, error)
	// RunOutput runs a command and returns stdout only.
	RunOutput(name string, args ...string) (string, error)
}

// defaultRunner delegates to the system package wrappers.
type defaultRunner struct{}

func (defaultRunner) RunSilent(name string, args ...string) (string, error) {
	return system.RunCommandSilent(name, args...)
}

func (defaultRunner) RunOutput(name string, args ...string) (string, error) {
	return system.RunCommandOutput(name, args...)
}

// Doctor holds configuration for running diagnostic checks.
type Doctor struct {
	Runner  CommandRunner
	Version string // current CLI version for update check
}

// New returns a Doctor with the default system runner.
func New(version string) *Doctor {
	return &Doctor{
		Runner:  defaultRunner{},
		Version: version,
	}
}

// RunAll executes every check and prints results via ui helpers.
// Returns the summary so the CLI layer can set the exit code.
func (d *Doctor) RunAll() Summary {
	ui.Header("OpenBoot Doctor")

	results := []Result{
		d.CheckBrew(),
		d.CheckBrewOutdated(),
		d.CheckGit(),
		d.CheckNode(),
		d.CheckNpm(),
		d.CheckShell(),
		d.CheckOhMyZsh(),
		d.CheckBrewShellenv(),
		d.CheckOpenBootDir(),
		d.CheckOpenBootState(),
		d.CheckPATH(),
	}

	ui.Muted("") // blank line before summary
	var s Summary
	for _, r := range results {
		switch r.Status {
		case StatusOK:
			s.Passed++
		case StatusWarn:
			s.Warnings++
		case StatusFail:
			s.Errors++
		}
	}

	parts := []string{fmt.Sprintf("%d passed", s.Passed)}
	if s.Warnings > 0 {
		parts = append(parts, fmt.Sprintf("%d warning(s)", s.Warnings))
	}
	if s.Errors > 0 {
		parts = append(parts, fmt.Sprintf("%d error(s)", s.Errors))
	}
	ui.Info(strings.Join(parts, ", "))
	return s
}

// ── Individual checks ────────────────────────────────────────────────────────

// CheckBrew verifies Homebrew is installed and reachable.
func (d *Doctor) CheckBrew() Result {
	path, err := exec.LookPath("brew")
	if err != nil {
		return fail("Homebrew not found (install from https://brew.sh)")
	}
	return ok(fmt.Sprintf("Homebrew installed (%s)", path))
}

// CheckBrewOutdated reports the number of outdated Homebrew packages.
func (d *Doctor) CheckBrewOutdated() Result {
	if _, err := exec.LookPath("brew"); err != nil {
		return warn("Skipped outdated-package check (Homebrew not installed)")
	}
	output, err := d.Runner.RunOutput("brew", "outdated", "--quiet")
	if err != nil {
		return warn(fmt.Sprintf("Could not check outdated packages: %v", err))
	}
	output = strings.TrimSpace(output)
	if output == "" {
		return ok("All Homebrew packages up to date")
	}
	count := len(strings.Split(output, "\n"))
	return warn(fmt.Sprintf("%d outdated Homebrew package(s) (run `brew upgrade` to update)", count))
}

// CheckGit verifies git is installed and user identity is configured.
func (d *Doctor) CheckGit() Result {
	if _, err := exec.LookPath("git"); err != nil {
		return fail("Git not found")
	}
	name, _ := d.Runner.RunSilent("git", "config", "--global", "user.name")
	email, _ := d.Runner.RunSilent("git", "config", "--global", "user.email")
	name = strings.TrimSpace(name)
	email = strings.TrimSpace(email)
	if name == "" || email == "" {
		return warn("Git installed but user.name or user.email not configured")
	}
	return ok(fmt.Sprintf("Git configured (user.name: %q, user.email: %q)", name, email))
}

// CheckNode verifies Node.js is available.
func (d *Doctor) CheckNode() Result {
	if _, err := exec.LookPath("node"); err != nil {
		return fail("Node.js not found (install via `openboot install` or `brew install node`)")
	}
	ver, _ := d.Runner.RunOutput("node", "--version")
	ver = strings.TrimSpace(ver)
	if ver != "" {
		return ok(fmt.Sprintf("Node.js %s", ver))
	}
	return ok("Node.js installed")
}

// CheckNpm verifies npm is available.
func (d *Doctor) CheckNpm() Result {
	if _, err := exec.LookPath("npm"); err != nil {
		return fail("npm not found (usually installed with Node.js)")
	}
	ver, _ := d.Runner.RunOutput("npm", "--version")
	ver = strings.TrimSpace(ver)
	if ver != "" {
		return ok(fmt.Sprintf("npm %s", ver))
	}
	return ok("npm installed")
}

// CheckShell verifies the default shell is zsh.
func (d *Doctor) CheckShell() Result {
	shell := os.Getenv("SHELL")
	if strings.HasSuffix(shell, "/zsh") {
		return ok("Default shell: zsh")
	}
	if shell == "" {
		return warn("Could not determine default shell ($SHELL is empty)")
	}
	return warn(fmt.Sprintf("Default shell is %s, not zsh", filepath.Base(shell)))
}

// CheckOhMyZsh checks if Oh-My-Zsh is installed.
func (d *Doctor) CheckOhMyZsh() Result {
	home, err := os.UserHomeDir()
	if err != nil {
		return warn(fmt.Sprintf("Could not determine home directory: %v", err))
	}
	omzDir := filepath.Join(home, ".oh-my-zsh")
	if info, err := os.Stat(omzDir); err == nil && info.IsDir() {
		return ok("Oh-My-Zsh installed")
	}
	return warn("Oh-My-Zsh not found (~/.oh-my-zsh)")
}

// CheckBrewShellenv checks if ~/.zshrc sources brew shellenv on Apple Silicon.
func (d *Doctor) CheckBrewShellenv() Result {
	if runtime.GOARCH != "arm64" {
		return ok("Brew shellenv check not needed (Intel Mac)")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return warn(fmt.Sprintf("Could not determine home directory: %v", err))
	}
	zshrc := filepath.Join(home, ".zshrc")
	data, err := os.ReadFile(zshrc)
	if err != nil {
		return warn("~/.zshrc not found")
	}
	if strings.Contains(string(data), "brew shellenv") {
		return ok("~/.zshrc sources brew shellenv")
	}
	return warn("~/.zshrc does not source brew shellenv (Apple Silicon requires this)")
}

// CheckOpenBootDir checks if the ~/.openboot directory exists.
func (d *Doctor) CheckOpenBootDir() Result {
	home, err := os.UserHomeDir()
	if err != nil {
		return warn(fmt.Sprintf("Could not determine home directory: %v", err))
	}
	dir := filepath.Join(home, ".openboot")
	if info, err := os.Stat(dir); err == nil && info.IsDir() {
		return ok("~/.openboot directory exists")
	}
	return warn("~/.openboot directory not found (run `openboot install` to create it)")
}

// CheckOpenBootState checks for a saved sync source (indicates previous install).
func (d *Doctor) CheckOpenBootState() Result {
	home, err := os.UserHomeDir()
	if err != nil {
		return warn(fmt.Sprintf("Could not determine home directory: %v", err))
	}
	syncPath := filepath.Join(home, ".openboot", "sync_source.json")
	if _, err := os.Stat(syncPath); err == nil {
		return ok("Sync source found (previous install detected)")
	}
	statePath := filepath.Join(home, ".openboot", "state.json")
	if _, err := os.Stat(statePath); err == nil {
		return ok("State file found")
	}
	return warn("No install state found (run `openboot install` to get started)")
}

// CheckPATH verifies that /opt/homebrew/bin is in PATH on Apple Silicon.
func (d *Doctor) CheckPATH() Result {
	if runtime.GOARCH != "arm64" {
		return ok("PATH check not needed (Intel Mac)")
	}
	pathEnv := os.Getenv("PATH")
	if strings.Contains(pathEnv, "/opt/homebrew/bin") {
		return ok("/opt/homebrew/bin is in PATH")
	}
	return warn("/opt/homebrew/bin is not in PATH (Apple Silicon requires this for Homebrew)")
}

// ── Helpers ──────────────────────────────────────────────────────────────────

func ok(msg string) Result {
	ui.Success(msg)
	return Result{Status: StatusOK, Message: msg}
}

func warn(msg string) Result {
	ui.Warn(msg)
	return Result{Status: StatusWarn, Message: msg}
}

func fail(msg string) Result {
	ui.Error(msg)
	return Result{Status: StatusFail, Message: msg}
}
