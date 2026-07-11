package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/openbootdotdev/openboot/internal/installer"
)

// splitPostInstall must remove the post-install script from the plan streamed
// through the wizard pipeline (the alt-screen can't host its confirm) while
// preserving it for the after-teardown run. This is the R1 regression: the
// pipeline path forced plan.Silent=true, which silently gated out post-install
// execution that the linear path used to preview, confirm, and run.
func TestSplitPostInstall(t *testing.T) {
	plan := installer.InstallPlan{
		Formulae:    []string{"jq"},
		PostInstall: []string{"echo hi", "echo bye"},
	}

	streamed, deferred := splitPostInstall(plan)

	assert.Nil(t, streamed.PostInstall, "streamed plan must not carry post-install")
	assert.Equal(t, []string{"echo hi", "echo bye"}, deferred, "post-install deferred to after teardown")
	assert.Equal(t, []string{"jq"}, streamed.Formulae, "the rest of the plan is untouched")
	assert.Equal(t, []string{"echo hi", "echo bye"}, plan.PostInstall, "the caller's plan is not mutated")
}

func TestSplitPostInstall_Empty(t *testing.T) {
	streamed, deferred := splitPostInstall(installer.InstallPlan{Formulae: []string{"jq"}})
	assert.Empty(t, deferred)
	assert.Equal(t, []string{"jq"}, streamed.Formulae)
}
