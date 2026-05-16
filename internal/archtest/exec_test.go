package archtest

import "testing"

// execAllowedPaths is the set of files/packages allowed to call os/exec
// directly. Everything else must use internal/system or a documented runner.
// Adding a path here is an intentional architectural decision — review the
// rule in CLAUDE.md ("Subprocess") before extending.
var execAllowedPaths = []string{
	"internal/system",         // canonical generic runner
	"internal/brew/runner.go", // brew runner — wrapped, fakeable
	"internal/npm/runner.go",  // npm runner — wrapped, fakeable
}

// TestNoDirectExec enforces the CLAUDE.md rule:
//
//	Do not call exec.Command directly from feature code —
//	add to system/ if a wrapper is missing.
func TestNoDirectExec(t *testing.T) {
	r := rule{
		name: "no-direct-exec",
		fix:  "Call internal/system.RunCommand or RunCommandSilent instead of exec.Command. If you genuinely need a new wrapper, add it to internal/system and call that.",
	}
	var violations []callSite
	for _, gf := range productionFiles(t) {
		if inAllowedPath(gf.path, execAllowedPaths) {
			continue
		}
		violations = append(violations, findCall(gf, "os/exec", "Command")...)
		violations = append(violations, findCall(gf, "os/exec", "CommandContext")...)
	}
	enforce(t, r, violations)
}
