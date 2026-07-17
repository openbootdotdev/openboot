package archtest

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// bareReadRe matches a `read` prompt that takes no explicit input redirect.
// `read ... </dev/tty` and `read ... <<<"$x"` are fine; a plain `read` is not.
var bareReadRe = regexp.MustCompile(`(^|\s|;|\||&&)read\s`)

// TestInstallShHasNoBareRead enforces the rule that scripts/install.sh must
// never prompt on stdin.
//
// Its headline use is:
//
//	curl -fsSL openboot.dev/install.sh | bash
//
// There, stdin is the pipe carrying the script itself — not the user's
// keyboard. A bare `read` consumes the script's own next bytes as the answer,
// so it both mangles the script and takes a branch the user never chose.
//
// This is not hypothetical. The "OpenBoot is already installed. Reinstall?
// (y/N)" prompt defaulted to No, and via the documented curl|bash it always
// took that default — silently keeping every existing user on their old
// version, then exec'ing it. Releases went out that nobody could receive, and
// the symptom (`openboot version` reporting yesterday's build after a
// successful-looking install) pointed nowhere near the cause.
//
// Prompts must read from /dev/tty via the script's ask_tty helper, which also
// supplies a default for when there is no terminal at all (CI, piped shells).
func TestInstallShHasNoBareRead(t *testing.T) {
	path := filepath.Join("..", "..", "scripts", "install.sh")
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read install.sh: %v", err)
	}

	var bad []string
	for i, line := range strings.Split(string(src), "\n") {
		code, _, _ := strings.Cut(line, "#") // ignore prose in comments
		if !bareReadRe.MatchString(code) {
			continue
		}
		// The helper's own read is the sanctioned one: it redirects </dev/tty.
		if strings.Contains(code, "/dev/tty") {
			continue
		}
		bad = append(bad, "scripts/install.sh:"+itoa(i+1)+": "+strings.TrimSpace(line))
	}

	if len(bad) > 0 {
		t.Errorf("install.sh prompts on stdin, which under `curl | bash` is the script itself:\n  %s\n\n"+
			"Fix: ask via the ask_tty helper (reads /dev/tty, falls back to a default with no terminal).",
			strings.Join(bad, "\n  "))
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
