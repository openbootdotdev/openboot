package archtest

import (
	"go/ast"
	"strings"
	"testing"
)

// dryRunExemptPaths lists packages that are exempt from the dry-run guard
// rule. These are infrastructure packages where the callers (not the
// packages themselves) are responsible for checking DryRun.
var dryRunExemptPaths = []string{
	"internal/archtest", // the rules themselves
	"internal/ui",       // output helpers
	"internal/logging",  // log file management
	"internal/state",    // internal state persistence
	"internal/updater",  // self-update; runs before user commands
	"internal/system",   // wrappers; callers are responsible for dry-run
	"internal/httputil", // network; not destructive to local state
	"internal/config",   // reads config + writes cache files
	"internal/auth",     // login/logout; not gated by dry-run by design
	"cmd/",              // main entry point, not destructive
}

// dryRunExemptFiles lists individual files exempt from the rule.
var dryRunExemptFiles = []string{
	"internal/installer/state.go",     // install state tracking
	"internal/snapshot/capture.go",    // read-only system probes (brew list, npm list, git config --get, etc.)
	"internal/snapshot/dock.go",       // read-only: `defaults export | plutil` for Dock pinned apps
	"internal/snapshot/loginitems.go", // read-only: osascript reads Login Items
	"internal/sync/diff.go",           // read-only dotfiles remote probe for diff computation
}

// destructiveOsCalls lists os package functions that modify the filesystem.
var destructiveOsCalls = []string{
	"WriteFile",
	"Remove",
	"RemoveAll",
	"Rename",
	"MkdirAll",
	"Create",
	"OpenFile",
}

// destructiveSystemCalls lists system package functions that run subprocesses.
var destructiveSystemCalls = []string{
	"RunCommand",
	"RunCommandSilent",
	"RunCommandOutput",
}

// hasDryRunReference reports whether the function body contains any
// reference to a variable or field named dryRun or DryRun (case-insensitive
// prefix match on "dryrun"/"DryRun").
func hasDryRunReference(fn *ast.FuncDecl) bool {
	if fn.Body == nil {
		return false
	}

	// Check function parameters for dryRun/DryRun.
	if fn.Type.Params != nil {
		for _, field := range fn.Type.Params.List {
			for _, name := range field.Names {
				low := strings.ToLower(name.Name)
				if low == "dryrun" {
					return true
				}
			}
		}
	}

	// Walk the function body looking for any identifier or selector
	// referencing dryRun/DryRun.
	found := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if found {
			return false
		}
		switch v := n.(type) {
		case *ast.Ident:
			low := strings.ToLower(v.Name)
			if low == "dryrun" {
				found = true
				return false
			}
		case *ast.SelectorExpr:
			low := strings.ToLower(v.Sel.Name)
			if low == "dryrun" {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

// findDestructiveWithoutDryRun walks each function declaration in gf and
// reports functions that contain destructive calls but lack a dryRun/DryRun
// reference. Each destructive call site inside such a function is reported.
func findDestructiveWithoutDryRun(gf goFile) []callSite {
	osLocal := importedAs(gf.file, "os")
	sysLocal := importedAs(gf.file, "github.com/openbootdotdev/openboot/internal/system")

	var out []callSite
	for _, decl := range gf.file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		if hasDryRunReference(fn) {
			continue
		}

		// Collect destructive call sites inside this function.
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			ident, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}

			matched := false
			if osLocal != "" && ident.Name == osLocal {
				for _, fn := range destructiveOsCalls {
					if sel.Sel.Name == fn {
						matched = true
						break
					}
				}
			}
			if sysLocal != "" && ident.Name == sysLocal {
				for _, fn := range destructiveSystemCalls {
					if sel.Sel.Name == fn {
						matched = true
						break
					}
				}
			}
			if matched {
				p := gf.fset.Position(call.Pos())
				out = append(out, callSite{file: gf.path, line: p.Line})
			}
			return true
		})
	}
	return out
}

// TestDryRunGuard enforces the CLAUDE.md rule:
//
//	Destructive ops: check cfg.DryRun first. Always.
//
// Functions that call destructive operations (system.RunCommand*,
// os.WriteFile, os.Remove*, os.Rename, os.MkdirAll, os.Create, os.OpenFile)
// must reference dryRun/DryRun somewhere in their body — either as a
// parameter or a field access (e.g. cfg.DryRun). Infrastructure packages
// are exempt; the baseline captures known exceptions.
func TestDryRunGuard(t *testing.T) {
	r := rule{
		name: "dryrun",
		fix:  "Add a dryRun bool parameter or check cfg.DryRun before calling destructive operations (system.RunCommand*, os.WriteFile, os.Remove, etc.). If this function is infrastructure that doesn't need a dry-run guard, add it to the baseline.",
	}
	var violations []callSite
	for _, gf := range productionFiles(t) {
		if inAllowedPath(gf.path, dryRunExemptPaths) {
			continue
		}
		exempt := false
		for _, ef := range dryRunExemptFiles {
			if gf.path == ef {
				exempt = true
				break
			}
		}
		if exempt {
			continue
		}
		violations = append(violations, findDestructiveWithoutDryRun(gf)...)
	}
	enforce(t, r, violations)
}
