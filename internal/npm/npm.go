package npm

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/openbootdotdev/openboot/internal/ui"
)

func IsAvailable() bool {
	_, err := exec.LookPath("npm")
	return err == nil
}

func GetNodeVersion() (int, error) {
	cmd := exec.Command("node", "--version")
	output, err := cmd.Output()
	if err != nil {
		return 0, err
	}

	version := strings.TrimSpace(string(output))
	version = strings.TrimPrefix(version, "v")
	parts := strings.Split(version, ".")
	if len(parts) == 0 || parts[0] == "" {
		return 0, fmt.Errorf("invalid version format")
	}

	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("invalid version format: %w", err)
	}

	return major, nil
}

func GetInstalledPackages() (map[string]bool, error) {
	packages := make(map[string]bool)

	cmd := exec.Command("npm", "list", "-g", "--depth=0", "--parseable")
	output, err := cmd.Output()
	if err != nil {
		if len(output) > 0 {
			// npm list exits non-zero when packages have issues, but still
			// provides parseable output — treat as success
		} else {
			return packages, fmt.Errorf("npm list -g: %w", err)
		}
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) <= 1 {
		return packages, nil
	}
	for _, line := range lines[1:] {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "/")
		pkgName := parts[len(parts)-1]
		if len(parts) >= 2 && strings.HasPrefix(parts[len(parts)-2], "@") {
			pkgName = parts[len(parts)-2] + "/" + parts[len(parts)-1]
		}
		if pkgName != "" && pkgName != "npm" && pkgName != "corepack" {
			packages[pkgName] = true
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

	nodeVersion, err := GetNodeVersion()
	if err == nil && nodeVersion > 0 {
		packagesNeedingNode22 := []string{"wrangler", "@cloudflare/wrangler"}
		needsWarning := false
		for _, pkg := range packages {
			for _, needNode22 := range packagesNeedingNode22 {
				if pkg == needNode22 {
					needsWarning = true
					break
				}
			}
			if needsWarning {
				break
			}
		}

		if needsWarning && nodeVersion < 22 {
			ui.Warn(fmt.Sprintf("Node.js v%d detected. Some packages (like wrangler) require Node.js v22+", nodeVersion))
			ui.Muted("Consider upgrading Node.js: brew install node@22")
			fmt.Println()
		}
	}

	if dryRun {
		ui.Info("Would install npm packages:")
		for _, p := range packages {
			fmt.Printf("    npm install -g %s\n", p)
		}
		return nil
	}

	installed, err := GetInstalledPackages()
	if err != nil {
		return fmt.Errorf("list installed packages: %w", err)
	}

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

	ui.Info(fmt.Sprintf("Installing %d npm packages...", len(toInstall)))

	args := append([]string{"install", "-g"}, toInstall...)
	cmd := exec.Command("npm", args...)
	batchOutput, err := cmd.CombinedOutput()

	var failed []string
	if err == nil {
		ui.Success(fmt.Sprintf("  ✔ %d npm packages installed", len(toInstall)))
	} else {
		batchError := parseNpmError(string(batchOutput))
		ui.Warn(fmt.Sprintf("Batch install failed (%s), falling back to sequential...", batchError))
		fmt.Println()

		nowInstalled, err := GetInstalledPackages()
		if err != nil {
			return fmt.Errorf("list packages after batch: %w", err)
		}

		var remaining []string
		for _, pkg := range toInstall {
			if !nowInstalled[pkg] {
				remaining = append(remaining, pkg)
			}
		}

		if len(remaining) == 0 {
			ui.Success("All npm packages already installed after partial batch!")
		} else {
			progress := ui.NewStickyProgress(len(remaining))
			progress.Start()

			for _, pkg := range remaining {
				progress.SetCurrent(pkg)
				errMsg := installNpmPackageWithRetry(pkg)
				if errMsg != "" {
					progress.PrintLine("  ✗ %s (%s)", pkg, errMsg)
					failed = append(failed, pkg)
				} else {
					progress.PrintLine("  ✔ %s", pkg)
				}
				progress.Increment()
			}

			progress.Finish()
		}
	}

	if len(failed) > 0 {
		fmt.Println()
		ui.Error(fmt.Sprintf("%d npm packages failed to install:", len(failed)))
		for _, f := range failed {
			fmt.Printf("    - %s\n", f)
		}
		return fmt.Errorf("%d packages failed to install", len(failed))
	}

	return nil
}

func Uninstall(packages []string, dryRun bool) error {
	if len(packages) == 0 {
		return nil
	}

	if !IsAvailable() {
		ui.Warn("npm not found — skipping npm package removal")
		return nil
	}

	if dryRun {
		ui.Info("Would uninstall npm packages:")
		for _, p := range packages {
			fmt.Printf("    npm uninstall -g %s\n", p)
		}
		return nil
	}

	var failed []string
	for _, pkg := range packages {
		cmd := exec.Command("npm", "uninstall", "-g", pkg)
		if output, err := cmd.CombinedOutput(); err != nil {
			ui.Warn(fmt.Sprintf("Failed to uninstall %s: %s", pkg, strings.TrimSpace(string(output))))
			failed = append(failed, pkg)
		} else {
			ui.Success(fmt.Sprintf("  ✔ Uninstalled %s", pkg))
		}
	}

	if len(failed) > 0 {
		return fmt.Errorf("%d npm packages failed to uninstall", len(failed))
	}
	return nil
}

func installNpmPackageWithRetry(pkg string) string {
	maxAttempts := 3
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		cmd := exec.Command("npm", "install", "-g", pkg)
		output, err := cmd.CombinedOutput()
		if err == nil {
			return ""
		}

		errMsg := parseNpmError(string(output))
		if attempt < maxAttempts && isNpmRetryableError(errMsg) {
			delay := time.Duration(attempt) * 2 * time.Second
			time.Sleep(delay)
			continue
		}

		return errMsg
	}
	return "max retries exceeded"
}

func isNpmRetryableError(errMsg string) bool {
	retryableErrors := []string{
		"network error",
		"connection",
		"timeout",
	}
	for _, retryable := range retryableErrors {
		if strings.Contains(strings.ToLower(errMsg), retryable) {
			return true
		}
	}
	return false
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
		lines := strings.Split(strings.TrimSpace(output), "\n")
		lastLine := strings.TrimSpace(lines[len(lines)-1])
		if lastLine != "" && len(lastLine) < 120 {
			return lastLine
		}
		return "install failed"
	}
}
