package brew

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/openbootdotdev/openboot/internal/ui"
)

const maxWorkers = 4

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
		return nil, err
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

func InstallWithProgress(cliPkgs, caskPkgs []string, dryRun bool) error {
	total := len(cliPkgs) + len(caskPkgs)
	if total == 0 {
		return nil
	}

	if dryRun {
		ui.Info("Would install packages:")
		for _, p := range cliPkgs {
			fmt.Printf("    brew install %s\n", p)
		}
		for _, p := range caskPkgs {
			fmt.Printf("    brew install --cask %s\n", p)
		}
		return nil
	}

	installedFormulae, installedCasks, err := GetInstalledPackages()
	if err != nil {
		return fmt.Errorf("failed to check installed packages: %w", err)
	}

	var newCli []string
	for _, p := range cliPkgs {
		if !installedFormulae[p] {
			newCli = append(newCli, p)
		}
	}
	var newCask []string
	for _, p := range caskPkgs {
		if !installedCasks[p] {
			newCask = append(newCask, p)
		}
	}

	skipped := total - len(newCli) - len(newCask)
	if skipped > 0 {
		ui.Muted(fmt.Sprintf("  %d already installed, %d to install", skipped, len(newCli)+len(newCask)))
		fmt.Println()
	}

	if len(newCli)+len(newCask) == 0 {
		ui.Success("All packages already installed!")
		return nil
	}

	if err := PreInstallChecks(len(newCli) + len(newCask)); err != nil {
		return err
	}

	progress := ui.NewStickyProgress(len(newCli) + len(newCask))
	progress.SetSkipped(skipped)
	progress.Start()

	var allFailed []failedJob

	if len(newCli) > 0 {
		failed := runParallelInstallWithProgress(newCli, progress)
		allFailed = append(allFailed, failed...)
	}

	if len(newCask) > 0 {
		for _, pkg := range newCask {
			progress.SetCurrent(pkg)
			progress.PrintLine("  Installing %s...", pkg)
			start := time.Now()
			errMsg := installCaskWithProgress(pkg, progress)
			elapsed := time.Since(start)
			progress.IncrementWithStatus(errMsg == "")
			duration := ui.FormatDuration(elapsed)
			if errMsg == "" {
				progress.PrintLine("  %s %s", ui.Green("✔ "+pkg), ui.Cyan("("+duration+")"))
			} else {
				progress.PrintLine("  %s %s", ui.Red("✗ "+pkg+" ("+errMsg+")"), ui.Cyan("("+duration+")"))
			}
			if errMsg != "" {
				allFailed = append(allFailed, failedJob{
					installJob: installJob{name: pkg, isCask: true},
					errMsg:     errMsg,
				})
			}
		}
	}

	progress.Finish()

	if len(allFailed) > 0 {
		fmt.Printf("\nRetrying %d failed packages...\n", len(allFailed))

		retriedSuccessfully := make(map[string]bool)

		for _, f := range allFailed {
			var errMsg string
			if f.isCask {
				errMsg = installSmartCaskWithError(f.name)
			} else {
				errMsg = installFormulaWithError(f.name)
			}
			if errMsg == "" {
				fmt.Printf("  ✔ %s (retry succeeded)\n", f.name)
				retriedSuccessfully[f.name] = true
			} else {
				fmt.Printf("  ✗ %s (still failed)\n", f.name)
			}
		}

		var stillFailed []failedJob
		for _, f := range allFailed {
			if !retriedSuccessfully[f.name] {
				stillFailed = append(stillFailed, f)
			}
		}
		allFailed = stillFailed
	}

	handleFailedJobs(allFailed)

	return nil
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

func runParallelInstallWithProgress(pkgs []string, progress *ui.StickyProgress) []failedJob {
	if len(pkgs) == 0 {
		return nil
	}

	jobs := make([]installJob, 0, len(pkgs))
	for _, pkg := range pkgs {
		jobs = append(jobs, installJob{name: pkg, isCask: false})
	}

	jobChan := make(chan installJob, len(jobs))
	results := make(chan installResult, len(jobs))

	var wg sync.WaitGroup
	workers := maxWorkers
	if len(jobs) < workers {
		workers = len(jobs)
	}

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobChan {
				progress.SetCurrent(job.name)
				start := time.Now()
				errMsg := installFormulaWithError(job.name)
				elapsed := time.Since(start)
				progress.IncrementWithStatus(errMsg == "")
				duration := ui.FormatDuration(elapsed)
				if errMsg == "" {
					progress.PrintLine("  %s %s", ui.Green("✔ "+job.name), ui.Cyan("("+duration+")"))
				} else {
					progress.PrintLine("  %s %s", ui.Red("✗ "+job.name+" ("+errMsg+")"), ui.Cyan("("+duration+")"))
				}
				results <- installResult{name: job.name, failed: errMsg != "", isCask: job.isCask, errMsg: errMsg}
			}
		}()
	}

	go func() {
		for _, job := range jobs {
			jobChan <- job
		}
		close(jobChan)
	}()

	go func() {
		wg.Wait()
		close(results)
	}()

	var failed []failedJob
	for result := range results {
		if result.failed {
			failed = append(failed, failedJob{
				installJob: installJob{name: result.name, isCask: result.isCask},
				errMsg:     result.errMsg,
			})
		}
	}

	return failed
}

func installCaskWithProgress(pkg string, progress *ui.StickyProgress) string {
	progress.PauseForInteractive()

	cmd := exec.Command("brew", "install", "--cask", pkg)
	cmd.Stdin = os.Stdin
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

func installFormulaWithError(pkg string) string {
	cmd := exec.Command("brew", "install", pkg)
	output, err := cmd.CombinedOutput()
	if err != nil {
		outputStr := string(output)
		if strings.Contains(strings.ToLower(outputStr), "try again using") && strings.Contains(strings.ToLower(outputStr), "--cask") {
			cmd2 := exec.Command("brew", "install", "--cask", pkg)
			output2, err2 := cmd2.CombinedOutput()
			if err2 != nil {
				return parseBrewError(string(output2))
			}
			return ""
		}
		return parseBrewError(outputStr)
	}
	return ""
}

func installSmartCaskWithError(pkg string) string {
	cmd := exec.Command("brew", "install", "--cask", pkg)
	output, err := cmd.CombinedOutput()
	if err != nil {
		cmd2 := exec.Command("brew", "install", pkg)
		output2, err2 := cmd2.CombinedOutput()
		if err2 != nil {
			errMsg := parseBrewError(string(output))
			if errMsg == "unknown error" {
				errMsg = parseBrewError(string(output2))
			}
			return errMsg
		}
	}
	return ""
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

func installSmartCask(pkg string) error {
	cmd := exec.Command("brew", "install", "--cask", pkg)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		cmd2 := exec.Command("brew", "install", pkg)
		cmd2.Stdout = nil
		cmd2.Stderr = nil
		return cmd2.Run()
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
		return err
	}

	ui.Info("Upgrading packages...")
	cmd := exec.Command("brew", "upgrade")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
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
			return fmt.Errorf("failed to close connection to %s: %w", host, err)
		}
	}
	return nil
}

func CheckDiskSpace() (float64, error) {
	var stat syscall.Statfs_t
	home, err := os.UserHomeDir()
	if err != nil {
		return 0, fmt.Errorf("cannot determine home directory: %w", err)
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
	if err != nil {
		return nil, fmt.Errorf("brew doctor failed: %w", err)
	}
	outputStr := string(output)

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
		suggestions = append(suggestions, "Run 'brew doctor' to see full diagnostic output")
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

	return nil
}
