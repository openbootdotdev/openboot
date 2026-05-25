package archtest

import (
	"go/ast"
	"testing"
)

// fmtPrintAllowedPaths lists packages that are allowed to use fmt.Print* calls
// directly. internal/ui owns the output helpers and internal/archtest itself
// uses fmt.Fprintf to write baselines.
var fmtPrintAllowedPaths = []string{
	"internal/ui",       // owns the output helpers
	"internal/archtest", // uses fmt.Fprintf internally for baseline writes
}

// isOsStderr reports whether the AST expression represents os.Stderr.
// Matches the selector expression os.Stderr, regardless of how "os" is
// aliased in the import block (we check the local name via importedAs).
func isOsStderr(gf goFile, expr ast.Expr) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	if sel.Sel.Name != "Stderr" {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	local := importedAs(gf.file, "os")
	return local != "" && ident.Name == local
}

// findFmtPrint finds forbidden fmt.Print*/fmt.F* calls in gf.
//   - fmt.Print, fmt.Println, fmt.Printf: always forbidden (stdout only).
//   - fmt.Fprint, fmt.Fprintln, fmt.Fprintf: forbidden unless first argument
//     is os.Stderr (stderr is acceptable for debug/error output).
func findFmtPrint(gf goFile) []callSite {
	local := importedAs(gf.file, "fmt")
	if local == "" {
		return nil
	}

	// stdoutFuncs always write to stdout — always forbidden.
	stdoutFuncs := map[string]bool{
		"Print":   true,
		"Println": true,
		"Printf":  true,
	}
	// writerFuncs take an io.Writer as first arg — forbidden unless that writer
	// is os.Stderr.
	writerFuncs := map[string]bool{
		"Fprint":   true,
		"Fprintln": true,
		"Fprintf":  true,
	}

	var out []callSite
	ast.Inspect(gf.file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		ident, ok := sel.X.(*ast.Ident)
		if !ok || ident.Name != local {
			return true
		}
		fn := sel.Sel.Name
		if stdoutFuncs[fn] {
			p := gf.fset.Position(call.Pos())
			out = append(out, callSite{file: gf.path, line: p.Line})
			return true
		}
		if writerFuncs[fn] {
			// Only flag if the first argument is NOT os.Stderr.
			if len(call.Args) == 0 || !isOsStderr(gf, call.Args[0]) {
				p := gf.fset.Position(call.Pos())
				out = append(out, callSite{file: gf.path, line: p.Line})
			}
			return true
		}
		return true
	})
	return out
}

// TestNoRawFmtPrint enforces the CLAUDE.md rule:
//
//	UI output must always go through ui.* helpers; raw fmt.Print* calls
//	are bugs in user-facing paths.
//
// fmt.Fprintf/Fprintln/Fprint to os.Stderr are exempt — stderr is acceptable
// for debug/error output.
func TestNoRawFmtPrint(t *testing.T) {
	r := rule{
		name: "fmtprint",
		fix:  "Use ui.* helpers (e.g. ui.Println, ui.Printf) instead of fmt.Print*. If writing to os.Stderr for debug/error output, use fmt.Fprintf(os.Stderr, ...) which is exempt.",
	}
	var violations []callSite
	for _, gf := range productionFiles(t) {
		if inAllowedPath(gf.path, fmtPrintAllowedPaths) {
			continue
		}
		violations = append(violations, findFmtPrint(gf)...)
	}
	enforce(t, r, violations)
}
