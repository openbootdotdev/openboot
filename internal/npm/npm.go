package npm

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/openbootdotdev/openboot/internal/ui"
)

func IsAvailable() bool {
	_, err := exec.LookPath("npm")
	return err == nil
}

func GetInstalledPackages() (map[string]bool, error) {
	packages := make(map[string]bool)

	cmd := exec.Command("npm", "list", "-g", "--depth=0", "--parseable")
	output, err := cmd.Output()
	if err != nil {
		return packages, nil
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
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
				packages[pkgName] = true
			}
		}
	}

	return packages, nil
}

func Install(packages []string, dryRun bool) error {
	if len(packages) == 0 {
		return nil
	}

	if !IsAvailable() {
		ui.Warn("npm not found — skipping npm packages (install Node.js first)")
		return nil
	}

	if dryRun {
		ui.Info("Would install npm packages:")
		for _, p := range packages {
			fmt.Printf("    npm install -g %s\n", p)
		}
		return nil
	}

	installed, _ := GetInstalledPackages()

	var toInstall []string
	for _, p := range packages {
		if !installed[p] {
			toInstall = append(toInstall, p)
		}
	}

	skipped := len(packages) - len(toInstall)
	if skipped > 0 {
		ui.Muted(fmt.Sprintf("  %d already installed, %d to install", skipped, len(toInstall)))
		fmt.Println()
	}

	if len(toInstall) == 0 {
		ui.Success("All npm packages already installed!")
		return nil
	}

	progress := ui.NewStickyProgress(len(toInstall))
	progress.Start()

	var failed []string
	for _, pkg := range toInstall {
		progress.SetCurrent(pkg)
		cmd := exec.Command("npm", "install", "-g", pkg)
		output, err := cmd.CombinedOutput()
		if err != nil {
			progress.PrintLine("  ✗ %s (%s)", pkg, parseNpmError(string(output)))
			failed = append(failed, pkg)
		} else {
			progress.PrintLine("  ✔ %s", pkg)
		}
		progress.Increment()
	}

	progress.Finish()

	if len(failed) > 0 {
		fmt.Println()
		ui.Error(fmt.Sprintf("%d npm packages failed to install:", len(failed)))
		for _, f := range failed {
			fmt.Printf("    - %s\n", f)
		}
	}

	return nil
}

func parseNpmError(output string) string {
	lowerOutput := strings.ToLower(output)
	switch {
	case strings.Contains(lowerOutput, "404 not found"):
		return "package not found"
	case strings.Contains(lowerOutput, "eacces"):
		return "permission denied"
	case strings.Contains(lowerOutput, "enetwork") || strings.Contains(lowerOutput, "enotfound"):
		return "network error"
	case strings.Contains(lowerOutput, "enospc"):
		return "disk full"
	default:
		return "install failed"
	}
}
