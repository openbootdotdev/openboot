package brew

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/openbootdotdev/openboot/internal/system"
	"github.com/openbootdotdev/openboot/internal/ui"
)

// checkNetworkFunc is a test seam for network connectivity checks.
var checkNetworkFunc = CheckNetwork

type OutdatedPackage struct {
	Name    string
	Current string
	Latest  string
}

func IsInstalled() bool {
	_, err := exec.LookPath("brew")
	return err == nil
}

func GetInstalledPackages() (formulae map[string]bool, casks map[string]bool, err error) {
	formulae = make(map[string]bool)
	casks = make(map[string]bool)

	var fOut, cOut []byte
	var fErr, cErr error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		fOut, fErr = currentRunner().Output("list", "--formula", "-1")
	}()
	go func() {
		defer wg.Done()
		cOut, cErr = currentRunner().Output("list", "--cask", "-1")
	}()
	wg.Wait()

	if fErr != nil {
		return nil, nil, fmt.Errorf("list formulae: %w", fErr)
	}
	if cErr != nil {
		return nil, nil, fmt.Errorf("list casks: %w", cErr)
	}

	for _, name := range strings.Split(strings.TrimSpace(string(fOut)), "\n") {
		if name != "" {
			formulae[name] = true
		}
	}
	for _, name := range strings.Split(strings.TrimSpace(string(cOut)), "\n") {
		if name != "" {
			casks[name] = true
		}
	}
	return
}

func ListOutdated() ([]OutdatedPackage, error) {
	output, err := currentRunner().Output("outdated", "--json")
	if err != nil {
		return nil, fmt.Errorf("brew outdated: %w", err)
	}

	var result struct {
		Formulae []struct {
			Name              string   `json:"name"`
			InstalledVersions []string `json:"installed_versions"`
			CurrentVersion    string   `json:"current_version"`
		} `json:"formulae"`
		Casks []struct {
			Name              string   `json:"name"`
			InstalledVersions []string `json:"installed_versions"`
			CurrentVersion    string   `json:"current_version"`
		} `json:"casks"`
	}

	if err := json.Unmarshal(output, &result); err != nil {
		return nil, err
	}

	var outdated []OutdatedPackage
	for _, f := range result.Formulae {
		current := ""
		if len(f.InstalledVersions) > 0 {
			current = f.InstalledVersions[0]
		}
		outdated = append(outdated, OutdatedPackage{
			Name:    f.Name,
			Current: current,
			Latest:  f.CurrentVersion,
		})
	}
	for _, c := range result.Casks {
		current := ""
		if len(c.InstalledVersions) > 0 {
			current = c.InstalledVersions[0]
		}
		outdated = append(outdated, OutdatedPackage{
			Name:    c.Name + " (cask)",
			Current: current,
			Latest:  c.CurrentVersion,
		})
	}

	return outdated, nil
}

func Uninstall(packages []string, dryRun bool) error {
	if len(packages) == 0 {
		return nil
	}

	if dryRun {
		ui.Info("Would uninstall CLI packages:")
		for _, p := range packages {
			fmt.Printf("    brew uninstall %s\n", p)
		}
		return nil
	}

	var failed []string
	for _, pkg := range packages {
		if output, err := currentRunner().CombinedOutput("uninstall", pkg); err != nil {
			ui.Warn(fmt.Sprintf("Failed to uninstall %s: %s", pkg, strings.TrimSpace(string(output))))
			failed = append(failed, pkg)
		} else {
			ui.Success(fmt.Sprintf("  ✔ Uninstalled %s", pkg))
		}
	}

	if len(failed) > 0 {
		return fmt.Errorf("%d formulae failed to uninstall", len(failed))
	}
	return nil
}

func UninstallCask(packages []string, dryRun bool) error {
	if len(packages) == 0 {
		return nil
	}

	if dryRun {
		ui.Info("Would uninstall GUI applications:")
		for _, p := range packages {
			fmt.Printf("    brew uninstall --cask %s\n", p)
		}
		return nil
	}

	var failed []string
	for _, pkg := range packages {
		if output, err := currentRunner().CombinedOutput("uninstall", "--cask", pkg); err != nil {
			ui.Warn(fmt.Sprintf("Failed to uninstall %s: %s", pkg, strings.TrimSpace(string(output))))
			failed = append(failed, pkg)
		} else {
			ui.Success(fmt.Sprintf("  ✔ Uninstalled %s (cask)", pkg))
		}
	}

	if len(failed) > 0 {
		return fmt.Errorf("%d casks failed to uninstall", len(failed))
	}
	return nil
}

func Untap(taps []string, dryRun bool) error {
	if len(taps) == 0 {
		return nil
	}

	if dryRun {
		ui.Info("Would remove taps:")
		for _, t := range taps {
			fmt.Printf("    brew untap %s\n", t)
		}
		return nil
	}

	var failed []string
	for _, tap := range taps {
		if output, err := currentRunner().CombinedOutput("untap", tap); err != nil {
			ui.Warn(fmt.Sprintf("Failed to remove tap %s: %s", tap, strings.TrimSpace(string(output))))
			failed = append(failed, tap)
		} else {
			ui.Success(fmt.Sprintf("  ✔ Removed tap %s", tap))
		}
	}

	if len(failed) > 0 {
		return fmt.Errorf("%d tap(s) failed to remove", len(failed))
	}
	return nil
}

func Update(dryRun bool) error {
	if dryRun {
		ui.Info("Would run: brew update && brew upgrade")
		return nil
	}

	ui.Info("Updating Homebrew...")
	if err := currentRunner().Run("update"); err != nil {
		return fmt.Errorf("brew update: %w", err)
	}

	ui.Info("Upgrading packages...")
	// Upgrade may prompt for sudo; RunInteractive wires the current TTY.
	if err := currentRunner().RunInteractive("upgrade"); err != nil {
		return fmt.Errorf("brew upgrade: %w", err)
	}
	return nil
}

func Cleanup() error {
	ui.Info("Cleaning up old versions...")
	return currentRunner().Run("cleanup")
}

func CheckNetwork() error {
	hosts := []string{"github.com:443", "raw.githubusercontent.com:443"}
	for _, host := range hosts {
		conn, err := net.DialTimeout("tcp", host, 5*time.Second)
		if err != nil {
			return fmt.Errorf("cannot reach %s: %v", host, err)
		}
		if err := conn.Close(); err != nil {
			return fmt.Errorf("close %s: %w", host, err)
		}
	}
	return nil
}

func CheckDiskSpace() (float64, error) {
	var stat syscall.Statfs_t
	home, err := system.HomeDir()
	if err != nil {
		return 0, err
	}
	if err := syscall.Statfs(home, &stat); err != nil {
		return 0, err
	}
	// stat.Bsize is uint32 on darwin and int64 on linux; the kernel always
	// reports a non-negative block size so the conversion is safe.
	availableGB := float64(stat.Bavail*uint64(stat.Bsize)) / (1024 * 1024 * 1024) //nolint:gosec // see comment above
	return availableGB, nil
}

func DoctorDiagnose() ([]string, error) {
	output, err := currentRunner().CombinedOutput("doctor")
	outputStr := string(output)
	// brew doctor exits non-zero when it finds warnings — that's expected.
	// Only treat as a hard error if the process couldn't start at all.
	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		return nil, fmt.Errorf("brew doctor failed: %w", err)
	}

	if strings.Contains(outputStr, "ready to brew") {
		return nil, nil
	}

	var suggestions []string
	lowerOutput := strings.ToLower(outputStr)

	if strings.Contains(lowerOutput, "unbrewed header files") {
		suggestions = append(suggestions, "Run: sudo rm -rf /usr/local/include")
	}
	if strings.Contains(lowerOutput, "unbrewed dylibs") {
		suggestions = append(suggestions, "Run: brew doctor --list-checks and review linked libraries")
	}
	if strings.Contains(lowerOutput, "homebrew/core") && strings.Contains(lowerOutput, "tap") {
		suggestions = append(suggestions, "Run: brew untap homebrew/core homebrew/cask")
	}
	if strings.Contains(lowerOutput, "git origin remote") {
		suggestions = append(suggestions, "Run: brew update-reset")
	}
	if strings.Contains(lowerOutput, "broken symlinks") {
		suggestions = append(suggestions, "Run: brew cleanup --prune=all")
	}
	if strings.Contains(lowerOutput, "outdated xcode") || strings.Contains(lowerOutput, "command line tools") {
		suggestions = append(suggestions, "Run: xcode-select --install")
	}
	if strings.Contains(lowerOutput, "uncommitted modifications") {
		suggestions = append(suggestions, "Run: brew update-reset")
	}
	if strings.Contains(lowerOutput, "permission") {
		suggestions = append(suggestions, "Run: sudo chown -R $(whoami) $(brew --prefix)/*")
	}

	if len(suggestions) == 0 && !strings.Contains(outputStr, "ready to brew") {
		suggestions = append(suggestions, "Run: brew doctor (to see full diagnostic output)")
	}

	return suggestions, nil
}

func PreInstallChecks(packageCount int) error {
	ui.Info("Checking network connectivity...")
	if err := checkNetworkFunc(); err != nil {
		return fmt.Errorf("network check failed: %v\nPlease check your internet connection and try again", err)
	}

	estimatedGB := float64(packageCount) * 0.2
	if estimatedGB < 1.0 {
		estimatedGB = 1.0
	}

	availableGB, err := CheckDiskSpace()
	if err == nil {
		if availableGB < estimatedGB {
			return fmt.Errorf("insufficient disk space: %.1f GB available, estimated %.1f GB needed\nFree up disk space and try again", availableGB, estimatedGB)
		}
		if availableGB < 5.0 {
			ui.Warn(fmt.Sprintf("Low disk space: %.1f GB available. Consider freeing up space.", availableGB))
		}
	}

	ui.Info("Updating Homebrew index...")
	if err := currentRunner().RunInteractive("update"); err != nil {
		ui.Warn("brew update failed, continuing anyway...")
	}

	return nil
}
