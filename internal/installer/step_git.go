package installer

import (
	"fmt"

	"github.com/openbootdotdev/openboot/internal/system"
	"github.com/openbootdotdev/openboot/internal/ui"
)

func applyGitConfig(plan InstallPlan, r Reporter) error {
	existingName, existingEmail := system.GetExistingGitConfig()
	if existingName != "" && existingEmail != "" {
		r.Success(fmt.Sprintf("Already configured: %s <%s>", existingName, existingEmail))
		ui.Println()
		return nil
	}

	if plan.GitName == "" || plan.GitEmail == "" {
		r.Muted("Git identity not available in snapshot, skipping")
		ui.Println()
		return nil
	}

	if plan.DryRun {
		ui.DryRunMsg("Would configure git: %s <%s>", plan.GitName, plan.GitEmail)
	} else {
		if err := system.ConfigureGit(plan.GitName, plan.GitEmail); err != nil {
			return fmt.Errorf("configure git: %w", err)
		}
		r.Success(fmt.Sprintf("Git configured: %s <%s>", plan.GitName, plan.GitEmail))
	}

	ui.Println()
	return nil
}
