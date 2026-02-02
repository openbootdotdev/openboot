package brew

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/fullstackjam/openboot/internal/ui"
)

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

	progress := ui.NewProgressTracker(total)
	var failed []string

	for _, pkg := range cliPkgs {
		progress.SetCurrent(pkg)
		cmd := exec.Command("brew", "install", pkg)
		cmd.Stdout = nil
		cmd.Stderr = nil
		if err := cmd.Run(); err != nil {
			failed = append(failed, pkg)
		}
		progress.Complete(pkg)
	}

	for _, pkg := range caskPkgs {
		progress.SetCurrent(pkg)
		if installSmartCask(pkg) != nil {
			failed = append(failed, pkg)
		}
		progress.Complete(pkg)
	}

	progress.Finish()

	if len(failed) > 0 {
		ui.Muted(fmt.Sprintf("Note: %d packages failed to install: %v", len(failed), failed))
	}

	return nil
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
