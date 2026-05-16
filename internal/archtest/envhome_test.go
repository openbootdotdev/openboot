package archtest

import "testing"

// TestNoOsGetenvHome enforces the CLAUDE.md rule:
//
//	Paths — os.UserHomeDir() — never hardcode `~` or `/Users/...`.
//
// Reading $HOME via os.Getenv is brittle on macOS — os.UserHomeDir wraps the
// platform-appropriate lookup. The baseline file is empty by design: this is
// a hard rule. If a violation is ever intentional, regenerating the baseline
// is still possible via ARCHTEST_UPDATE_BASELINE=1, but the diff is the
// audit trail and reviewers should push back.
func TestNoOsGetenvHome(t *testing.T) {
	r := rule{
		name: "no-os-getenv-home",
		fix:  `Use os.UserHomeDir() instead of os.Getenv("HOME").`,
	}
	var violations []callSite
	for _, gf := range productionFiles(t) {
		violations = append(violations, find(gf, usage{
			pkgPath:     "os",
			ident:       "Getenv",
			requireCall: true,
			stringArg0:  "HOME",
		})...)
	}
	enforce(t, r, violations)
}
