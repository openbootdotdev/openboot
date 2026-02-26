package dotfiles

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/openbootdotdev/openboot/internal/system"
	"github.com/openbootdotdev/openboot/internal/ui"
)

const defaultDotfilesDir = ".dotfiles"

func Clone(repoURL string, dryRun bool) error {
	if repoURL == "" {
		return nil
	}

	home, err := system.HomeDir()
	if err != nil {
		return err
	}
	dotfilesPath := filepath.Join(home, defaultDotfilesDir)

	if _, err := os.Stat(dotfilesPath); err == nil {
		gitDir := filepath.Join(dotfilesPath, ".git")
		if _, err := os.Stat(gitDir); os.IsNotExist(err) {
			fmt.Printf("Dotfiles already exist at %s, skipping clone\n", dotfilesPath)
			return nil
		}

		// Check whether the remote URL has changed.
		remoteChanged := false
		var currentURL string
		if out, err := exec.Command("git", "-C", dotfilesPath, "remote", "get-url", "origin").Output(); err == nil {
			currentURL = strings.TrimSpace(string(out))
			remoteChanged = currentURL != repoURL
		}

		if remoteChanged {
			if dryRun {
				fmt.Printf("[DRY-RUN] Would backup %s and re-clone from %s\n", dotfilesPath, repoURL)
				return nil
			}
			// Back up the old repo and fall through to a fresh clone.
			backupPath := dotfilesPath + ".openboot.bak"
			fmt.Printf("Dotfiles remote changed from %s to %s, backing up to %s and re-cloning\n", currentURL, repoURL, backupPath)
			if err := os.Rename(dotfilesPath, backupPath); err != nil {
				return fmt.Errorf("failed to backup existing dotfiles: %w", err)
			}
		} else {
			if dryRun {
				fmt.Printf("[DRY-RUN] Would sync latest dotfiles at %s\n", dotfilesPath)
				return nil
			}
			fmt.Printf("Dotfiles already exist at %s, syncing latest changes\n", dotfilesPath)
			// Use fetch + reset instead of pull to handle dirty states
			// (unmerged files, mid-rebase, etc.) gracefully.
			fetchCmd := exec.Command("git", "-C", dotfilesPath, "fetch", "origin")
			fetchCmd.Stdout = os.Stdout
			fetchCmd.Stderr = os.Stderr
			if err := fetchCmd.Run(); err != nil {
				return err
			}
			branchOut, err := exec.Command("git", "-C", dotfilesPath, "rev-parse", "--abbrev-ref", "HEAD").Output()
			if err != nil {
				return fmt.Errorf("failed to detect dotfiles branch: %w", err)
			}
			branch := strings.TrimSpace(string(branchOut))
			resetCmd := exec.Command("git", "-C", dotfilesPath, "reset", "--hard", "origin/"+branch)
			resetCmd.Stdout = os.Stdout
			resetCmd.Stderr = os.Stderr
			return resetCmd.Run()
		}
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
	home, err := system.HomeDir()
	if err != nil {
		return err
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

func backupFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read %s: %w", src, err)
	}
	if err := os.WriteFile(dst, data, 0644); err != nil {
		return fmt.Errorf("write backup %s: %w", dst, err)
	}
	return nil
}

func restoreFile(backup, original string) {
	if _, err := os.Stat(backup); os.IsNotExist(err) {
		return
	}
	if err := os.Rename(backup, original); err != nil {
		fmt.Printf("Warning: failed to restore %s from backup: %v\n", original, err)
	}
}

func linkWithStow(dotfilesPath string, dryRun bool) error {
	entries, err := os.ReadDir(dotfilesPath)
	if err != nil {
		return err
	}

	home, err := system.HomeDir()
	if err != nil {
		return err
	}

	var errs []error

	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		pkg := entry.Name()
		if dryRun {
			fmt.Printf("[DRY-RUN] Would stow package: %s\n", pkg)
			continue
		}

		// For the zsh package: back up .zshrc before removing it so we can
		// restore it if stow fails, preventing an unrecoverable loss of shell config.
		var zshrcBackedUp bool
		var zshrcPath, zshrcBackupPath string
		if pkg == "zsh" {
			zshrcPath = filepath.Join(home, ".zshrc")
			zshrcBackupPath = zshrcPath + ".openboot.bak"
			if _, statErr := os.Stat(zshrcPath); statErr == nil {
				if err := backupFile(zshrcPath, zshrcBackupPath); err != nil {
					errs = append(errs, fmt.Errorf("stow %s: %w", pkg, err))
					continue
				}
				zshrcBackedUp = true
			}
			os.Remove(zshrcPath)
			os.Remove(filepath.Join(home, ".zshrc.pre-oh-my-zsh"))
		}

		cmd := exec.Command("stow", "-v", "-t", home, pkg)
		cmd.Dir = dotfilesPath
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			// Restore .zshrc backup so the user isn't left without a shell config.
			if zshrcBackedUp {
				restoreFile(zshrcBackupPath, zshrcPath)
			}
			errs = append(errs, fmt.Errorf("stow %s: %w", pkg, err))
			continue
		}

		if zshrcBackedUp {
			if err := os.Remove(zshrcBackupPath); err != nil {
				ui.Warn(fmt.Sprintf("could not remove .zshrc backup: %v", err))
			}
		}
	}

	return errors.Join(errs...)
}

func linkDirect(dotfilesPath string, dryRun bool) error {
	home, err := system.HomeDir()
	if err != nil {
		return err
	}

	entries, err := os.ReadDir(dotfilesPath)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		name := entry.Name()
		// Only link dotfiles (entries starting with "."), skip .git itself.
		if !strings.HasPrefix(name, ".") || name == ".git" {
			continue
		}

		src := filepath.Join(dotfilesPath, name)
		dst := filepath.Join(home, name)

		if dryRun {
			fmt.Printf("[DRY-RUN] Would symlink %s -> %s\n", dst, src)
			continue
		}

		// Already correctly linked — nothing to do.
		if target, err := os.Readlink(dst); err == nil && target == src {
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
