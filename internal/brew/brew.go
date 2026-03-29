package brew

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/openbootdotdev/openboot/internal/system"
	"github.com/openbootdotdev/openboot/internal/ui"
)

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
		fOut, fErr = exec.Command("brew", "list", "--formula", "-1").Output()
	}()
	go func() {
		defer wg.Done()
		cOut, cErr = exec.Command("brew", "list", "--cask", "-1").Output()
	}()
	wg.Wait()

	if fErr != nil {
		return nil, nil, fErr
	}
	if cErr != nil {
		return nil, nil, cErr
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
	cmd := exec.Command("brew", "outdated", "--json")
	output, err := cmd.Output()
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

func Install(packages []string, dryRun bool) error {
	if len(packages) == 0 {
		return nil
	}

	if dryRun {
		ui.Info("Would install CLI packages:")
		for _, p := range packages {
			fmt.Printf("    brew install %s\n", p)
		}
		return nil
	}

	ui.Info(fmt.Sprintf("Installing %d CLI packages...", len(packages)))

	args := append([]string{"install"}, packages...)
	cmd := exec.Command("brew", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func InstallTaps(taps []string, dryRun bool) error {
	if len(taps) == 0 {
		return nil
	}

	if dryRun {
		ui.Info("Would add taps:")
		for _, t := range taps {
			fmt.Printf("    brew tap %s\n", t)
		}
		return nil
	}

	ui.Info(fmt.Sprintf("Adding %d Homebrew taps...", len(taps)))

	for _, tap := range taps {
		cmd := exec.Command("brew", "tap", tap)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			ui.Warn(fmt.Sprintf("Failed to tap %s: %v", tap, err))
		}
	}
	return nil
}

func InstallCask(packages []string, dryRun bool) error {
	if len(packages) == 0 {
		return nil
	}

	if dryRun {
		ui.Info("Would install GUI applications:")
		for _, p := range packages {
			fmt.Printf("    brew install --cask %s\n", p)
		}
		return nil
	}

	ui.Info(fmt.Sprintf("Installing %d GUI applications...", len(packages)))

	args := append([]string{"install", "--cask"}, packages...)
	cmd := exec.Command("brew", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

type installJob struct {
	name   string
	isCask bool
}

type installResult struct {
	name   string
	failed bool
	isCask bool
	errMsg string
}

func InstallWithProgress(cliPkgs, caskPkgs []string, dryRun bool) (installedFormulae []string, installedCasks []string, err error) {
	total := len(cliPkgs) + len(caskPkgs)
	if total == 0 {
		return nil, nil, nil
	}

	if dryRun {
		ui.Info("Would install packages:")
		if len(cliPkgs) > 0 {
			fmt.Printf("    brew install %s\n", strings.Join(cliPkgs, " "))
		}
		if len(caskPkgs) > 0 {
			fmt.Printf("    brew install --cask %s\n", strings.Join(caskPkgs, " "))
		}
		return nil, nil, nil
	}

	alreadyFormulae, alreadyCasks, checkErr := GetInstalledPackages()
	if checkErr != nil {
		return nil, nil, fmt.Errorf("list installed packages: %w", checkErr)
	}

	var newCli []string
	for _, p := range cliPkgs {
		if !alreadyFormulae[p] {
			newCli = append(newCli, p)
		} else {
			installedFormulae = append(installedFormulae, p)
		}
	}
	var newCask []string
	for _, p := range caskPkgs {
		if !alreadyCasks[p] {
			newCask = append(newCask, p)
		} else {
			installedCasks = append(installedCasks, p)
		}
	}

	skipped := total - len(newCli) - len(newCask)
	if skipped > 0 {
		ui.Muted(fmt.Sprintf("  %d already installed, %d to install", skipped, len(newCli)+len(newCask)))
		fmt.Println()
	}

	if len(newCli)+len(newCask) == 0 {
		ui.Success("All packages already installed!")
		return installedFormulae, installedCasks, nil
	}

	if preErr := PreInstallChecks(len(newCli) + len(newCask)); preErr != nil {
		return installedFormulae, installedCasks, preErr
	}

	progress := ui.NewStickyProgress(len(newCli) + len(newCask))
	progress.SetSkipped(skipped)
	progress.Start()

	var allFailed []failedJob

	if len(newCli) > 0 {
		ui.Info(fmt.Sprintf("Installing %d CLI packages...", len(newCli)))

		args := append([]string{"install"}, newCli...)
		cmd := brewInstallCmd(args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmdErr := cmd.Run()

		// Re-check installed packages to determine actual success
		postFormulae, _, postErr := GetInstalledPackages()
		if postErr != nil {
			// Fallback: use command error to determine status
			for _, pkg := range newCli {
				progress.IncrementWithStatus(cmdErr == nil)
				if cmdErr == nil {
					installedFormulae = append(installedFormulae, pkg)
				} else {
					allFailed = append(allFailed, failedJob{
						installJob: installJob{name: pkg, isCask: false},
						errMsg:     "install failed",
					})
				}
			}
		} else {
			// Check each package individually
			for _, pkg := range newCli {
				isInstalled := postFormulae[pkg]
				progress.IncrementWithStatus(isInstalled)
				if isInstalled {
					installedFormulae = append(installedFormulae, pkg)
				} else {
					allFailed = append(allFailed, failedJob{
						installJob: installJob{name: pkg, isCask: false},
						errMsg:     "install failed",
					})
				}
			}
		}
	}

	if len(newCask) > 0 {
		ui.Info(fmt.Sprintf("Installing %d GUI apps...", len(newCask)))

		args := append([]string{"install", "--cask"}, newCask...)
		cmd := brewInstallCmd(args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		// Open TTY for password prompts
		tty, opened := system.OpenTTY()
		if opened {
			cmd.Stdin = tty
		}
		cmdErr := cmd.Run()
		if opened {
			tty.Close()
		}

		// Re-check installed casks to determine actual success
		_, postCasks, postErr := GetInstalledPackages()
		if postErr != nil {
			// Fallback: use command error to determine status
			for _, pkg := range newCask {
				progress.IncrementWithStatus(cmdErr == nil)
				if cmdErr == nil {
					installedCasks = append(installedCasks, pkg)
				} else {
					allFailed = append(allFailed, failedJob{
						installJob: installJob{name: pkg, isCask: true},
						errMsg:     "install failed",
					})
				}
			}
		} else {
			// Check each cask individually
			for _, pkg := range newCask {
				isInstalled := postCasks[pkg]
				progress.IncrementWithStatus(isInstalled)
				if isInstalled {
					installedCasks = append(installedCasks, pkg)
				} else {
					allFailed = append(allFailed, failedJob{
						installJob: installJob{name: pkg, isCask: true},
						errMsg:     "install failed",
					})
				}
			}
		}
	}

	progress.Finish()

	if len(allFailed) > 0 {
		fmt.Printf("\nRetrying %d failed packages...\n", len(allFailed))

		for _, f := range allFailed {
			var errMsg string
			if f.isCask {
				errMsg = installSmartCaskWithError(f.name)
			} else {
				errMsg = installFormulaWithError(f.name)
			}
			if errMsg == "" {
				fmt.Printf("  ✔ %s (retry succeeded)\n", f.name)
				if f.isCask {
					installedCasks = append(installedCasks, f.name)
				} else {
					installedFormulae = append(installedFormulae, f.name)
				}
			} else {
				fmt.Printf("  ✗ %s (still failed)\n", f.name)
			}
		}

		type pkgKey struct {
			name   string
			isCask bool
		}
		succeeded := make(map[pkgKey]bool)
		for _, p := range installedFormulae {
			succeeded[pkgKey{p, false}] = true
		}
		for _, p := range installedCasks {
			succeeded[pkgKey{p, true}] = true
		}
		var stillFailed []failedJob
		for _, f := range allFailed {
			if !succeeded[pkgKey{f.name, f.isCask}] {
				stillFailed = append(stillFailed, f)
			}
		}
		allFailed = stillFailed
	}

	handleFailedJobs(allFailed)

	return installedFormulae, installedCasks, nil
}

func handleFailedJobs(failed []failedJob) {
	if len(failed) == 0 {
		return
	}

	fmt.Println()
	ui.Error(fmt.Sprintf("%d packages failed to install:", len(failed)))
	for _, f := range failed {
		if f.errMsg != "" {
			fmt.Printf("    - %s (%s)\n", f.name, f.errMsg)
		} else {
			fmt.Printf("    - %s\n", f.name)
		}
	}
}

type failedJob struct {
	installJob
	errMsg string
}

func installCaskWithProgress(pkg string, progress *ui.StickyProgress) string {
	progress.PauseForInteractive()

	cmd := brewInstallCmd("install", "--cask", pkg)
	tty, opened := system.OpenTTY()
	if opened {
		defer tty.Close()
	}
	cmd.Stdin = tty
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()

	progress.ResumeAfterInteractive()

	if err != nil {
		return "install failed"
	}
	return ""
}

func printBrewOutput(output string, progress *ui.StickyProgress) {
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			progress.PrintLine("    %s", line)
		}
	}
}

func brewInstallCmd(args ...string) *exec.Cmd {
	cmd := exec.Command("brew", args...)
	cmd.Env = append(os.Environ(), "HOMEBREW_NO_AUTO_UPDATE=1")
	return cmd
}

// brewCombinedOutputWithTTY runs a brew command capturing combined output while
// providing a TTY for stdin so that sudo password prompts work. The caller must
// close the returned *os.File if closeTTY is true.
func brewCombinedOutputWithTTY(args ...string) (string, error) {
	cmd := brewInstallCmd(args...)
	tty, opened := system.OpenTTY()
	if opened {
		cmd.Stdin = tty
		defer tty.Close()
	}
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func installFormulaWithError(pkg string) string {
	maxAttempts := 3
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		cmd := brewInstallCmd("install", pkg)
		output, err := cmd.CombinedOutput()
		if err == nil {
			return ""
		}

		outputStr := string(output)
		if strings.Contains(strings.ToLower(outputStr), "try again using") && strings.Contains(strings.ToLower(outputStr), "--cask") {
			// Cask installs may need sudo for .pkg — use TTY stdin.
			caskOutput, err2 := brewCombinedOutputWithTTY("install", "--cask", pkg)
			if err2 == nil {
				return ""
			}
			outputStr = caskOutput
		}

		errMsg := parseBrewError(outputStr)
		if attempt < maxAttempts && isRetryableError(errMsg) {
			delay := time.Duration(attempt) * 2 * time.Second
			time.Sleep(delay)
			continue
		}

		return errMsg
	}
	return "max retries exceeded"
}

func isRetryableError(errMsg string) bool {
	retryableErrors := []string{
		"connection timed out",
		"connection refused",
		"no internet connection",
		"download corrupted",
		"already running",
		"Cannot download non-corrupt",
		"signature mismatch",
	}
	lower := strings.ToLower(errMsg)
	for _, retryable := range retryableErrors {
		if strings.Contains(lower, strings.ToLower(retryable)) {
			return true
		}
	}
	return false
}

func installSmartCaskWithError(pkg string) string {
	maxAttempts := 3
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		// Cask installs may need sudo for .pkg — use TTY stdin.
		caskOutput, err := brewCombinedOutputWithTTY("install", "--cask", pkg)
		if err == nil {
			return ""
		}

		cmd2 := brewInstallCmd("install", pkg)
		output2, err2 := cmd2.CombinedOutput()
		if err2 == nil {
			return ""
		}

		errMsg := parseBrewError(caskOutput)
		if errMsg == "unknown error" {
			errMsg = parseBrewError(string(output2))
		}

		if attempt < maxAttempts && isRetryableError(errMsg) {
			delay := time.Duration(attempt) * 2 * time.Second
			time.Sleep(delay)
			continue
		}

		return errMsg
	}
	return "max retries exceeded"
}

func parseBrewError(output string) string {
	lowerOutput := strings.ToLower(output)

	switch {
	case strings.Contains(lowerOutput, "no available formula"):
		return "package not found"
	case strings.Contains(lowerOutput, "already installed"):
		return ""
	case strings.Contains(lowerOutput, "no internet"):
		return "no internet connection"
	case strings.Contains(lowerOutput, "connection refused"):
		return "connection refused"
	case strings.Contains(lowerOutput, "timed out"):
		return "connection timed out"
	case strings.Contains(lowerOutput, "permission denied"):
		return "permission denied"
	case strings.Contains(lowerOutput, "disk full") || strings.Contains(lowerOutput, "no space"):
		return "disk full"
	case strings.Contains(lowerOutput, "sha256 mismatch"):
		return "download corrupted"
	case strings.Contains(lowerOutput, "depends on"):
		return "dependency error"
	default:
		lines := strings.Split(output, "\n")
		for _, line := range lines {
			if strings.Contains(strings.ToLower(line), "error") {
				if len(line) > 60 {
					return line[:60] + "..."
				}
				return line
			}
		}
		return "unknown error"
	}
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
		cmd := exec.Command("brew", "uninstall", pkg)
		if output, err := cmd.CombinedOutput(); err != nil {
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
		cmd := exec.Command("brew", "uninstall", "--cask", pkg)
		if output, err := cmd.CombinedOutput(); err != nil {
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

// GetInstalledLeaves returns top-level formulae (not dependencies) as a set.
// This matches what `brew leaves` reports and is consistent with snapshot captures.
func GetInstalledLeaves() (map[string]bool, error) {
	output, err := exec.Command("brew", "leaves").Output()
	if err != nil {
		return nil, fmt.Errorf("brew leaves: %w", err)
	}

	leaves := make(map[string]bool)
	for _, name := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if name != "" {
			leaves[name] = true
		}
	}
	return leaves, nil
}

func GetInstalledTaps() ([]string, error) {
	cmd := exec.Command("brew", "tap")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("brew tap: %w", err)
	}
	var taps []string
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			taps = append(taps, line)
		}
	}
	return taps, nil
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
		cmd := exec.Command("brew", "untap", tap)
		if output, err := cmd.CombinedOutput(); err != nil {
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
	if err := exec.Command("brew", "update").Run(); err != nil {
		return fmt.Errorf("brew update: %w", err)
	}

	ui.Info("Upgrading packages...")
	cmd := exec.Command("brew", "upgrade")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	tty, opened := system.OpenTTY()
	if opened {
		defer tty.Close()
	}
	cmd.Stdin = tty
	return cmd.Run()
}

func Cleanup() error {
	ui.Info("Cleaning up old versions...")
	return exec.Command("brew", "cleanup").Run()
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
	availableGB := float64(stat.Bavail*uint64(stat.Bsize)) / (1024 * 1024 * 1024)
	return availableGB, nil
}

func DoctorDiagnose() ([]string, error) {
	cmd := exec.Command("brew", "doctor")
	output, err := cmd.CombinedOutput()
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
	if err := CheckNetwork(); err != nil {
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
	updateCmd := exec.Command("brew", "update")
	updateCmd.Stdout = nil
	updateCmd.Stderr = nil
	if err := updateCmd.Run(); err != nil {
		ui.Warn("brew update failed, continuing anyway...")
	}

	return nil
}

// ResolveFormulaName resolves a formula alias to its canonical name.
// This handles cases like "postgresql" → "postgresql@18" or "kubectl" → "kubernetes-cli".
// Returns the original name if resolution fails.
func ResolveFormulaName(name string) string {
	cmd := exec.Command("brew", "info", "--json", name)
	output, err := cmd.Output()
	if err != nil {
		return name
	}

	var result []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return name
	}

	if len(result) > 0 && result[0].Name != "" {
		return result[0].Name
	}
	return name
}
