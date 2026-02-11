package dotfiles

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const defaultDotfilesDir = ".dotfiles"

func Clone(repoURL string, dryRun bool) error {
	if repoURL == "" {
		return nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot determine home directory: %w", err)
	}
	dotfilesPath := filepath.Join(home, defaultDotfilesDir)

	if _, err := os.Stat(dotfilesPath); err == nil {
		fmt.Printf("Dotfiles already exist at %s, skipping clone\n", dotfilesPath)
		return nil
	}

	if dryRun {
		fmt.Printf("[DRY-RUN] Would clone %s to %s\n", repoURL, dotfilesPath)
		return nil
	}

	cmd := exec.Command("git", "clone", repoURL, dotfilesPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func Link(dryRun bool) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot determine home directory: %w", err)
	}
	dotfilesPath := filepath.Join(home, defaultDotfilesDir)

	if _, err := os.Stat(dotfilesPath); os.IsNotExist(err) {
		return fmt.Errorf("dotfiles directory not found: %s", dotfilesPath)
	}

	if hasStowPackages(dotfilesPath) {
		return linkWithStow(dotfilesPath, dryRun)
	}

	return linkDirect(dotfilesPath, dryRun)
}

func hasStowPackages(dotfilesPath string) bool {
	entries, err := os.ReadDir(dotfilesPath)
	if err != nil {
		return false
	}

	for _, entry := range entries {
		if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
			pkgPath := filepath.Join(dotfilesPath, entry.Name())
			subEntries, _ := os.ReadDir(pkgPath)
			for _, sub := range subEntries {
				if strings.HasPrefix(sub.Name(), ".") {
					return true
				}
			}
		}
	}
	return false
}

func linkWithStow(dotfilesPath string, dryRun bool) error {
	entries, err := os.ReadDir(dotfilesPath)
	if err != nil {
		return err
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot determine home directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		pkg := entry.Name()
		if dryRun {
			fmt.Printf("[DRY-RUN] Would stow package: %s\n", pkg)
			continue
		}

		if pkg == "zsh" {
			zshrc := filepath.Join(home, ".zshrc")
			zshrcBackup := filepath.Join(home, ".zshrc.pre-oh-my-zsh")
			os.Remove(zshrc)
			os.Remove(zshrcBackup)
		}

		cmd := exec.Command("stow", "-v", "-t", home, pkg)
		cmd.Dir = dotfilesPath
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Printf("Warning: failed to stow %s: %v\n", pkg, err)
		}
	}

	return nil
}

func linkDirect(dotfilesPath string, dryRun bool) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot determine home directory: %w", err)
	}

	entries, err := os.ReadDir(dotfilesPath)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		name := entry.Name()
		if name == ".git" || name == "README.md" || name == "LICENSE" {
			continue
		}

		src := filepath.Join(dotfilesPath, name)
		dst := filepath.Join(home, name)

		if dryRun {
			fmt.Printf("[DRY-RUN] Would symlink %s -> %s\n", dst, src)
			continue
		}

		if _, err := os.Lstat(dst); err == nil {
			backupPath := dst + ".openboot.bak"
			if err := os.Rename(dst, backupPath); err != nil {
				fmt.Printf("Warning: failed to backup %s: %v\n", dst, err)
				continue
			}
			fmt.Printf("Backed up: %s -> %s\n", dst, backupPath)
		}

		if err := os.Symlink(src, dst); err != nil {
			fmt.Printf("Warning: failed to symlink %s: %v\n", name, err)
		} else {
			fmt.Printf("Linked: %s -> %s\n", dst, src)
		}
	}

	return nil
}

func GetDotfilesURL() string {
	return os.Getenv("OPENBOOT_DOTFILES")
}
