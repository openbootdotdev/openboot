package installer

import (
	"fmt"

	"github.com/openbootdotdev/openboot/internal/config"
	"github.com/openbootdotdev/openboot/internal/system"
	"github.com/openbootdotdev/openboot/internal/ui"
)

func stepGitConfig(cfg *config.Config) error {
	ui.Header("Step 1: Git Configuration")
	fmt.Println()

	// Smart detection: skip if already configured
	existingName, existingEmail := system.GetExistingGitConfig()

	if existingName != "" && existingEmail != "" {
		ui.Success(fmt.Sprintf("✓ Already configured: %s <%s>", existingName, existingEmail))
		fmt.Println()
		return nil
	}

	var name, email string

	if cfg.DryRun && !system.HasTTY() {
		name = cfg.GitName
		email = cfg.GitEmail
		if name == "" {
			name = "Your Name"
		}
		if email == "" {
			email = "you@example.com"
		}
	} else if cfg.Silent {
		name = cfg.GitName
		email = cfg.GitEmail
		if name == "" || email == "" {
			return fmt.Errorf("OPENBOOT_GIT_NAME and OPENBOOT_GIT_EMAIL required in silent mode")
		}
	} else {
		var err error
		name, email, err = ui.InputGitConfig()
		if err != nil {
			return err
		}
	}

	if name == "" || email == "" {
		return fmt.Errorf("git name and email are required")
	}

	if cfg.DryRun {
		fmt.Printf("[DRY-RUN] Would configure git: %s <%s>\n", name, email)
	} else {
		if err := system.ConfigureGit(name, email); err != nil {
			return err
		}
		ui.Success(fmt.Sprintf("Git configured: %s <%s>", name, email))
	}

	fmt.Println()
	return nil
}

func stepRestoreGit(cfg *config.Config) error {
	ui.Header("Restore: Git Configuration")
	fmt.Println()

	git := cfg.SnapshotGit
	if git.UserName == "" && git.UserEmail == "" {
		ui.Muted("No git config in snapshot, skipping")
		fmt.Println()
		return nil
	}

	existingName, existingEmail := system.GetExistingGitConfig()

	if existingName != "" && existingEmail != "" {
		ui.Success(fmt.Sprintf("✓ Already configured: %s <%s>", existingName, existingEmail))
		fmt.Println()
		return nil
	}

	if cfg.DryRun {
		if existingName == "" && git.UserName != "" {
			fmt.Printf("[DRY-RUN] Would set git user.name = %s\n", git.UserName)
		}
		if existingEmail == "" && git.UserEmail != "" {
			fmt.Printf("[DRY-RUN] Would set git user.email = %s\n", git.UserEmail)
		}
		fmt.Println()
		return nil
	}

	nameToSet := existingName
	emailToSet := existingEmail
	if existingName == "" && git.UserName != "" {
		nameToSet = git.UserName
	}
	if existingEmail == "" && git.UserEmail != "" {
		emailToSet = git.UserEmail
	}

	if nameToSet == "" || emailToSet == "" {
		ui.Warn("Incomplete git config in snapshot, skipping (need both name and email)")
		fmt.Println()
		return nil
	}

	if nameToSet != existingName || emailToSet != existingEmail {
		if err := system.ConfigureGit(nameToSet, emailToSet); err != nil {
			return fmt.Errorf("restore git config: %w", err)
		}
	}

	ui.Success(fmt.Sprintf("Git restored: %s <%s>", nameToSet, emailToSet))
	fmt.Println()
	return nil
}
