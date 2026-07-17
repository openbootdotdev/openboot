package archtest

import (
	"go/ast"
	"testing"
)

// httpAllowedPaths is the set of packages allowed to perform raw HTTP
// round-trips or reference net/http globals. Everything else must route
// requests through internal/httputil.Do, which handles 429 + Retry-After
// uniformly. Request construction may remain local to call sites.
var httpAllowedPaths = []string{
	"internal/httputil", // owns the wrapper
}

// TestNoRawHTTPNewRequest enforces the AGENTS.md rule:
//
//	HTTP with retry — use httputil.Do() — handles 429 + Retry-After.
func TestNoRawHTTPNewRequest(t *testing.T) {
	r := rule{
		name: "no-raw-http",
		fix:  "Use internal/httputil.Do for HTTP round-trips — it handles 429 + Retry-After and per-call rate limiting. Keep request construction local; only the round-trip goes through httputil.",
	}
	var violations []callSite
	for _, gf := range productionFiles(t) {
		if inAllowedPath(gf.path, httpAllowedPaths) {
			continue
		}
		violations = append(violations, findRawHTTPDoCalls(gf)...)
		violations = append(violations, findRef(gf, "net/http", "DefaultClient")...)
		violations = append(violations, findRef(gf, "net/http", "DefaultTransport")...)
	}
	enforce(t, r, violations)
}

func findRawHTTPDoCalls(gf goFile) []callSite {
	if importedAs(gf.file, "net/http") == "" {
		return nil
	}

	httputilLocal := importedAs(gf.file, "github.com/openbootdotdev/openboot/internal/httputil")
	var out []callSite
	ast.Inspect(gf.file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Do" {
			return true
		}
		if len(call.Args) != 1 {
			return true
		}
		argIdent, ok := call.Args[0].(*ast.Ident)
		if !ok || argIdent.Name != "req" {
			return true
		}
		ident, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		if httputilLocal != "" && ident.Name == httputilLocal {
			return true
		}
		p := gf.fset.Position(call.Pos())
		out = append(out, callSite{file: gf.path, line: p.Line})
		return true
	})
	return out
}
