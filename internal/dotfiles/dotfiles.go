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

// DefaultDotfilesURL is the fallback dotfiles repository used when the user
// does not supply their own.
const DefaultDotfilesURL = "https://github.com/openbootdotdev/dotfiles"

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
			// Remove stale backup from a previous remote change to avoid rename failure.
			if _, err := os.Stat(backupPath); err == nil {
				if err := os.RemoveAll(backupPath); err != nil {
					return fmt.Errorf("remove stale dotfiles backup %s: %w", backupPath, err)
				}
			}
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
			branch := ""
			if out, err := exec.Command("git", "-C", dotfilesPath, "rev-parse", "--abbrev-ref", "HEAD").Output(); err == nil {
				branch = strings.TrimSpace(string(out))
			}
			// Detached HEAD (e.g. mid-rebase) or failed detection: resolve the remote's default branch.
			if branch == "" || branch == "HEAD" {
				if out, err := exec.Command("git", "-C", dotfilesPath, "symbolic-ref", "refs/remotes/origin/HEAD").Output(); err == nil {
					// Returns e.g. "refs/remotes/origin/main"
					ref := strings.TrimSpace(string(out))
					branch = strings.TrimPrefix(ref, "refs/remotes/origin/")
				}
			}
			if branch == "" || branch == "HEAD" {
				branch = "main"
			}
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
		if dryRun {
			fmt.Printf("[DRY-RUN] Would link dotfiles from %s\n", dotfilesPath)
			return nil
		}
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

func ensureStow(dryRun bool) error {
	if _, err := exec.LookPath("stow"); err == nil {
		return nil
	}
	if dryRun {
		fmt.Println("[DRY-RUN] Would install stow via Homebrew")
		return nil
	}
	ui.Info("Installing stow via Homebrew...")
	cmd := exec.Command("brew", "install", "stow")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to install stow: %w", err)
	}
	return nil
}

// backupConflicts walks a stow package directory and backs up any existing
// regular files in targetDir that would conflict with stow. Returns the list
// of backup pairs so they can be restored on failure or cleaned up on success.
func backupConflicts(pkgDir, targetDir string) ([][2]string, error) {
	var backed [][2]string

	err := filepath.WalkDir(pkgDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		rel, relErr := filepath.Rel(pkgDir, path)
		if relErr != nil {
			return relErr
		}
		target := filepath.Join(targetDir, rel)

		info, statErr := os.Lstat(target)
		if statErr != nil || info.Mode()&os.ModeSymlink != 0 {
			// Target doesn't exist or is already a symlink — no conflict.
			return nil
		}
		if info.IsDir() {
			return nil
		}

		backupPath := target + ".openboot.bak"
		if bErr := backupFile(target, backupPath); bErr != nil {
			return fmt.Errorf("backup %s: %w", target, bErr)
		}
		if rErr := os.Remove(target); rErr != nil {
			return fmt.Errorf("remove %s after backup: %w", target, rErr)
		}
		backed = append(backed, [2]string{backupPath, target})
		return nil
	})

	return backed, err
}

func linkWithStow(dotfilesPath string, dryRun bool) error {
	if err := ensureStow(dryRun); err != nil {
		return err
	}

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

		pkgDir := filepath.Join(dotfilesPath, pkg)

		// Back up any existing regular files that would conflict with stow.
		backed, backupErr := backupConflicts(pkgDir, home)
		if backupErr != nil {
			errs = append(errs, fmt.Errorf("stow %s: %w", pkg, backupErr))
			continue
		}

		// Remove Oh-My-Zsh leftover that also blocks the zsh package.
		if pkg == "zsh" {
			os.Remove(filepath.Join(home, ".zshrc.pre-oh-my-zsh"))
		}

		cmd := exec.Command("stow", "-v", "-t", home, pkg)
		cmd.Dir = dotfilesPath
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			// Restore all backups so the user isn't left without their config.
			for _, pair := range backed {
				restoreFile(pair[0], pair[1])
			}
			errs = append(errs, fmt.Errorf("stow %s: %w", pkg, err))
			continue
		}

		// Stow succeeded — clean up backups.
		for _, pair := range backed {
			if rmErr := os.Remove(pair[0]); rmErr != nil {
				ui.Warn(fmt.Sprintf("could not remove backup %s: %v", pair[0], rmErr))
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
		// Only link dotfiles (entries starting with "."), skip git metadata.
		if !strings.HasPrefix(name, ".") || name == ".git" || name == ".gitignore" || name == ".gitmodules" || name == ".gitattributes" {
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
