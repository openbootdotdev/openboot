package archtest

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
)

type callSite struct {
	file string // repo-relative, forward-slash
	line int
}

func (c callSite) String() string { return fmt.Sprintf("%s:%d", c.file, c.line) }

type goFile struct {
	fset *token.FileSet
	path string
	file *ast.File
}

// repoRoot is discovered via runtime.Caller — works regardless of test cwd,
// which os.Getwd-based discovery (like testutil.findProjectRoot) does not.
// Validated against the presence of go.mod so a future package move surfaces
// loudly instead of silently scanning the wrong tree.
func repoRoot() string {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		panic(fmt.Sprintf("archtest: repoRoot %q has no go.mod — did internal/archtest move?", root))
	}
	return root
}

var (
	parseOnce sync.Once
	cached    []goFile
)

// productionFiles parses every non-test .go file under internal/ and cmd/ in
// deterministic order, caching the result so multiple rules share one parse.
//
// Files that fail to parse are skipped with a log line rather than aborting —
// a build break would already fail the surrounding test suite, and partial
// results are more useful than none.
func productionFiles(t *testing.T) []goFile {
	t.Helper()
	parseOnce.Do(func() {
		root := repoRoot()
		fset := token.NewFileSet()
		for _, sub := range []string{"internal", "cmd"} {
			base := filepath.Join(root, sub)
			_ = filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
				if err != nil {
					t.Logf("archtest: walk %s: %v", p, err)
					return nil
				}
				if d.IsDir() {
					name := d.Name()
					if name == "vendor" || name == "testdata" {
						return fs.SkipDir
					}
					return nil
				}
				if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
					return nil
				}
				f, perr := parser.ParseFile(fset, p, nil, parser.SkipObjectResolution)
				if perr != nil {
					t.Logf("archtest: parse %s: %v (skipping)", p, perr)
					return nil
				}
				rel, _ := filepath.Rel(root, p)
				cached = append(cached, goFile{fset: fset, path: filepath.ToSlash(rel), file: f})
				return nil
			})
		}
	})
	return cached
}

// importedAs returns the local name the file uses for importPath, or "".
func importedAs(f *ast.File, importPath string) string {
	for _, imp := range f.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		if path != importPath {
			continue
		}
		if imp.Name != nil {
			return imp.Name.Name
		}
		idx := strings.LastIndex(path, "/")
		return path[idx+1:]
	}
	return ""
}

// usage describes a forbidden reference: `pkgPath.ident`, optionally
// restricted to call sites with a matching first-arg string literal.
type usage struct {
	pkgPath     string
	ident       string
	requireCall bool                // when false, also reports bare selector references (e.g. http.DefaultClient)
	stringArg0  string              // when requireCall && non-empty, the call must have args[0] == this string literal
}

// find returns positions in gf where u matches. Walks the AST once.
func find(gf goFile, u usage) []callSite {
	local := importedAs(gf.file, u.pkgPath)
	if local == "" {
		return nil
	}
	var out []callSite
	ast.Inspect(gf.file, func(n ast.Node) bool {
		var sel *ast.SelectorExpr
		var call *ast.CallExpr
		switch v := n.(type) {
		case *ast.CallExpr:
			call = v
			s, ok := v.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			sel = s
		case *ast.SelectorExpr:
			if u.requireCall {
				return true // calls are reported via the CallExpr branch
			}
			sel = v
		default:
			return true
		}
		ident, ok := sel.X.(*ast.Ident)
		if !ok || ident.Name != local || sel.Sel.Name != u.ident {
			return true
		}
		if u.stringArg0 != "" {
			if call == nil || len(call.Args) == 0 {
				return true
			}
			lit, ok := call.Args[0].(*ast.BasicLit)
			if !ok || strings.Trim(lit.Value, `"`) != u.stringArg0 {
				return true
			}
		}
		pos := sel.Pos()
		if call != nil {
			pos = call.Pos()
		}
		p := gf.fset.Position(pos)
		out = append(out, callSite{file: gf.path, line: p.Line})
		return true
	})
	return out
}

// findCall is a convenience wrapper for the common case of "call to pkg.ident".
func findCall(gf goFile, pkgPath, ident string) []callSite {
	return find(gf, usage{pkgPath: pkgPath, ident: ident, requireCall: true})
}

// findRef is a convenience wrapper for "any reference to pkg.ident", including
// non-call selectors like http.DefaultClient.
func findRef(gf goFile, pkgPath, ident string) []callSite {
	return find(gf, usage{pkgPath: pkgPath, ident: ident})
}

type rule struct {
	name string // matches the baseline file stem under baseline/
	fix  string // shown when a new violation is reported
}

func baselinePath(name string) string {
	return filepath.Join(repoRoot(), "internal", "archtest", "baseline", name+".txt")
}

func loadBaseline(t *testing.T, name string) map[string]bool {
	t.Helper()
	path := baselinePath(name)
	data, err := os.ReadFile(path) // #nosec G304 -- path is fully derived from a constant inside the repo
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]bool{}
		}
		t.Fatalf("read baseline %s: %v", path, err)
	}
	out := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if i := strings.Index(line, " #"); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		out[line] = true
	}
	return out
}

// sortCallSites sorts in place by (file, line).
func sortCallSites(s []callSite) {
	sort.Slice(s, func(i, j int) bool {
		if s[i].file != s[j].file {
			return s[i].file < s[j].file
		}
		return s[i].line < s[j].line
	})
}

func writeBaseline(t *testing.T, name string, found []callSite) {
	t.Helper()
	sortCallSites(found)
	var b strings.Builder
	fmt.Fprintf(&b, "# Baseline for archtest rule %q.\n", name)
	fmt.Fprintf(&b, "# Each line is <file>:<line> of a known existing violation.\n")
	fmt.Fprintf(&b, "# Regenerate: ARCHTEST_UPDATE_BASELINE=1 go test ./internal/archtest/...\n")
	for _, v := range found {
		fmt.Fprintln(&b, v.String())
	}
	path := baselinePath(name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir baseline dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil { // #nosec G306 -- baseline files are repo-tracked
		t.Fatalf("write baseline %s: %v", path, err)
	}
}

// enforce reports NEW violations as test failures and STALE baseline entries
// as log lines. Setting ARCHTEST_UPDATE_BASELINE=1 regenerates the baseline
// from the current set instead of comparing.
func enforce(t *testing.T, r rule, found []callSite) {
	t.Helper()
	if os.Getenv("ARCHTEST_UPDATE_BASELINE") == "1" {
		writeBaseline(t, r.name, found)
		t.Logf("archtest %s: baseline updated (%d entries)", r.name, len(found))
		return
	}

	baseline := loadBaseline(t, r.name)
	foundSet := map[string]bool{}
	var newViol []callSite
	for _, v := range found {
		foundSet[v.String()] = true
		if !baseline[v.String()] {
			newViol = append(newViol, v)
		}
	}
	var stale []string
	for k := range baseline {
		if !foundSet[k] {
			stale = append(stale, k)
		}
	}
	sort.Strings(stale)
	sortCallSites(newViol)

	if len(stale) > 0 {
		t.Logf("archtest %s: %d stale baseline entr(ies) — code was fixed but baseline not pruned (informational only):", r.name, len(stale))
		for _, s := range stale {
			t.Logf("  %s", s)
		}
	}
	if len(newViol) > 0 {
		var b strings.Builder
		fmt.Fprintf(&b, "\narchtest %s: %d new violation(s)\n\n", r.name, len(newViol))
		for _, v := range newViol {
			fmt.Fprintf(&b, "  %s\n", v.String())
		}
		fmt.Fprintf(&b, "\nFix: %s\n", r.fix)
		fmt.Fprintf(&b, "\nIf the change is intentional, regenerate the baseline:\n  ARCHTEST_UPDATE_BASELINE=1 go test ./internal/archtest/...\n")
		t.Fatal(b.String())
	}
}

// inAllowedPath reports whether path (repo-relative, forward slash) sits
// inside one of the allow entries. An entry is matched as either an exact
// file or a directory prefix.
func inAllowedPath(path string, allow []string) bool {
	for _, a := range allow {
		if path == a {
			return true
		}
		if strings.HasPrefix(path, strings.TrimSuffix(a, "/")+"/") {
			return true
		}
	}
	return false
}
