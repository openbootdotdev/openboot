package archtest

import "testing"

// httpAllowedPaths is the set of packages allowed to construct raw HTTP
// requests or reference net/http globals. Everything else must go through
// internal/httputil.Do, which handles 429 + Retry-After uniformly.
var httpAllowedPaths = []string{
	"internal/httputil", // owns the wrapper
}

// TestNoRawHTTPNewRequest enforces the CLAUDE.md rule:
//
//	HTTP with retry — use httputil.Do() — handles 429 + Retry-After.
func TestNoRawHTTPNewRequest(t *testing.T) {
	r := rule{
		name: "no-raw-http",
		fix:  "Use internal/httputil.Do for HTTP calls — it handles 429 + Retry-After and per-call rate limiting. Keep request construction local; only the round-trip goes through httputil.",
	}
	var violations []callSite
	for _, gf := range productionFiles(t) {
		if inAllowedPath(gf.path, httpAllowedPaths) {
			continue
		}
		violations = append(violations, findCall(gf, "net/http", "NewRequest")...)
		violations = append(violations, findCall(gf, "net/http", "NewRequestWithContext")...)
		violations = append(violations, findRef(gf, "net/http", "DefaultClient")...)
		violations = append(violations, findRef(gf, "net/http", "DefaultTransport")...)
	}
	enforce(t, r, violations)
}
