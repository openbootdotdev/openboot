package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/openbootdotdev/openboot/internal/brew"
	"github.com/openbootdotdev/openboot/internal/ui"
	"github.com/spf13/cobra"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check system health and diagnose issues",
	Long: `Run diagnostic checks on your development environment.

Checks performed:
- Homebrew installation and health
- Git configuration
- Shell configuration (Oh-My-Zsh)
- Common development tools
- Outdated packages`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDoctor()
	},
}

type checkResult struct {
	name    string
	status  string
	message string
}

func runDoctor() error {
	fmt.Println()
	ui.Header("OpenBoot Doctor")
	fmt.Println()

	var results []checkResult
	var issues int

	results = append(results, checkNetwork()...)
	results = append(results, checkDiskSpace()...)
	results = append(results, checkHomebrew()...)
	results = append(results, checkGit()...)
	results = append(results, checkShell()...)
	results = append(results, checkTools()...)

	for _, r := range results {
		switch r.status {
		case "ok":
			fmt.Printf("  %s %s\n", ui.Green("✓"), r.name)
		case "warn":
			fmt.Printf("  %s %s: %s\n", ui.Yellow("!"), r.name, r.message)
			issues++
		case "error":
			fmt.Printf("  %s %s: %s\n", ui.Red("✗"), r.name, r.message)
			issues++
		case "info":
			fmt.Printf("  %s %s: %s\n", ui.Cyan("i"), r.name, r.message)
		}
	}

	suggestions, _ := brew.DoctorDiagnose()
	if len(suggestions) > 0 {
		fmt.Println()
		ui.Info("Suggested fixes:")
		for _, s := range suggestions {
			fmt.Printf("    %s\n", s)
		}
	}

	fmt.Println()
	if issues == 0 {
		ui.Success("All checks passed! Your environment is healthy.")
	} else {
		ui.Muted(fmt.Sprintf("Found %d issue(s). Run 'openboot' to fix missing tools.", issues))
	}
	fmt.Println()

	return nil
}

func checkHomebrew() []checkResult {
	var results []checkResult

	_, err := exec.LookPath("brew")
	if err != nil {
		return []checkResult{{
			name:    "Homebrew",
			status:  "error",
			message: "not installed",
		}}
	}

	results = append(results, checkResult{
		name:   "Homebrew installed",
		status: "ok",
	})

	cmd := exec.Command("brew", "doctor")
	output, err := cmd.CombinedOutput()
	if err != nil {
		results = append(results, checkResult{
			name:    "Homebrew health",
			status:  "warn",
			message: "run 'brew doctor' for details",
		})
	} else if strings.Contains(string(output), "ready to brew") {
		results = append(results, checkResult{
			name:   "Homebrew health",
			status: "ok",
		})
	}

	cmd = exec.Command("brew", "outdated", "--json")
	output, _ = cmd.Output()
	if len(output) > 10 {
		count := strings.Count(string(output), "\"name\"")
		if count > 0 {
			results = append(results, checkResult{
				name:    "Outdated packages",
				status:  "info",
				message: fmt.Sprintf("%d packages can be upgraded (run 'openboot update')", count),
			})
		}
	}

	return results
}

func checkGit() []checkResult {
	var results []checkResult

	_, err := exec.LookPath("git")
	if err != nil {
		return []checkResult{{
			name:    "Git",
			status:  "error",
			message: "not installed",
		}}
	}

	results = append(results, checkResult{
		name:   "Git installed",
		status: "ok",
	})

	name, _ := exec.Command("git", "config", "--global", "user.name").Output()
	email, _ := exec.Command("git", "config", "--global", "user.email").Output()

	if len(strings.TrimSpace(string(name))) == 0 || len(strings.TrimSpace(string(email))) == 0 {
		results = append(results, checkResult{
			name:    "Git identity",
			status:  "warn",
			message: "user.name or user.email not configured",
		})
	} else {
		results = append(results, checkResult{
			name:   "Git identity",
			status: "ok",
		})
	}

	return results
}

func checkShell() []checkResult {
	var results []checkResult

	home, err := os.UserHomeDir()
	if err != nil {
		return []checkResult{{
			name:    "Shell",
			status:  "error",
			message: "cannot determine home directory",
		}}
	}
	omzPath := filepath.Join(home, ".oh-my-zsh")

	if _, err := os.Stat(omzPath); os.IsNotExist(err) {
		results = append(results, checkResult{
			name:    "Oh-My-Zsh",
			status:  "info",
			message: "not installed (optional)",
		})
	} else {
		results = append(results, checkResult{
			name:   "Oh-My-Zsh installed",
			status: "ok",
		})
	}

	zshrcPath := filepath.Join(home, ".zshrc")
	if _, err := os.Stat(zshrcPath); os.IsNotExist(err) {
		results = append(results, checkResult{
			name:    ".zshrc",
			status:  "info",
			message: "not found",
		})
	} else {
		results = append(results, checkResult{
			name:   ".zshrc exists",
			status: "ok",
		})
	}

	return results
}

func checkTools() []checkResult {
	var results []checkResult

	essentialTools := []string{"curl", "wget", "jq", "gh"}

	for _, tool := range essentialTools {
		if _, err := exec.LookPath(tool); err != nil {
			results = append(results, checkResult{
				name:    tool,
				status:  "info",
				message: "not installed",
			})
		}
	}

	return results
}

func checkNetwork() []checkResult {
	if err := brew.CheckNetwork(); err != nil {
		return []checkResult{{
			name:    "Network",
			status:  "error",
			message: "cannot reach GitHub (required for Homebrew)",
		}}
	}
	return []checkResult{{
		name:   "Network connectivity",
		status: "ok",
	}}
}

func checkDiskSpace() []checkResult {
	availableGB, err := brew.CheckDiskSpace()
	if err != nil {
		return nil
	}

	if availableGB < 1.0 {
		return []checkResult{{
			name:    "Disk space",
			status:  "error",
			message: fmt.Sprintf("critically low: %.1f GB available", availableGB),
		}}
	}
	if availableGB < 5.0 {
		return []checkResult{{
			name:    "Disk space",
			status:  "warn",
			message: fmt.Sprintf("low: %.1f GB available", availableGB),
		}}
	}
	return []checkResult{{
		name:   fmt.Sprintf("Disk space (%.0f GB free)", availableGB),
		status: "ok",
	}}
}
