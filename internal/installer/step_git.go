package installer

import (
	"fmt"

	"github.com/openbootdotdev/openboot/internal/system"
)

func applyGitConfig(plan InstallPlan, r Reporter) error {
	r.Header("Step 1: Git Configuration")
	fmt.Println()

	existingName, existingEmail := system.GetExistingGitConfig()
	if existingName != "" && existingEmail != "" {
		r.Success(fmt.Sprintf("✓ Already configured: %s <%s>", existingName, existingEmail))
		fmt.Println()
		return nil
	}

	if plan.GitName == "" || plan.GitEmail == "" {
		r.Muted("Git identity not available in snapshot, skipping")
		fmt.Println()
		return nil
	}

	if plan.DryRun {
		fmt.Printf("[DRY-RUN] Would configure git: %s <%s>\n", plan.GitName, plan.GitEmail)
	} else {
		if err := system.ConfigureGit(plan.GitName, plan.GitEmail); err != nil {
			return err
		}
		r.Success(fmt.Sprintf("Git configured: %s <%s>", plan.GitName, plan.GitEmail))
	}

	fmt.Println()
	return nil
}
