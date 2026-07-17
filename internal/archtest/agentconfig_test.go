package archtest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAgentConfigSharedAcrossTools keeps AGENTS.md and each SKILL.md as a
// single source of truth while exposing them through the native discovery
// paths used by Codex and Claude Code.
func TestAgentConfigSharedAcrossTools(t *testing.T) {
	root := repoRoot()

	claudeInstructions := filepath.Join(root, "CLAUDE.md")
	src, err := os.ReadFile(claudeInstructions) // #nosec G304 -- path is inside the repo
	if err != nil {
		t.Fatalf("read CLAUDE.md: %v", err)
	}
	if !hasExactLine(string(src), "@AGENTS.md") {
		t.Errorf("CLAUDE.md must import the canonical instructions with an exact @AGENTS.md line")
	}

	canonicalRoot := filepath.Join(root, ".agents", "skills")
	claudeRoot := filepath.Join(root, ".claude", "skills")
	canonicalNames := skillNames(t, canonicalRoot)
	claudeNames := skillNames(t, claudeRoot)

	for name := range canonicalNames {
		if !claudeNames[name] {
			t.Errorf("shared skill %q is missing its Claude Code adapter", name)
			continue
		}

		canonical := filepath.Join(canonicalRoot, name, "SKILL.md")
		adapter := filepath.Join(claudeRoot, name, "SKILL.md")
		info, err := os.Lstat(adapter)
		if err != nil {
			t.Errorf("inspect Claude Code adapter for skill %q: %v", name, err)
			continue
		}
		if info.Mode()&os.ModeSymlink == 0 {
			t.Errorf("%s must be a symlink to %s; do not copy shared skill instructions",
				repoRelative(root, adapter), repoRelative(root, canonical))
			continue
		}
		target, err := os.Readlink(adapter)
		if err != nil {
			t.Errorf("read Claude Code adapter for skill %q: %v", name, err)
			continue
		}
		if filepath.IsAbs(target) {
			t.Errorf("%s must use a relative symlink so it works in every checkout",
				repoRelative(root, adapter))
			continue
		}

		resolved, err := filepath.EvalSymlinks(adapter)
		if err != nil {
			t.Errorf("resolve Claude Code adapter for skill %q: %v", name, err)
			continue
		}
		if filepath.Clean(resolved) != filepath.Clean(canonical) {
			t.Errorf("%s resolves to %s, want %s",
				repoRelative(root, adapter), repoRelative(root, resolved), repoRelative(root, canonical))
		}
	}

	for name := range claudeNames {
		if !canonicalNames[name] {
			t.Errorf("Claude-only skill %q has no canonical .agents/skills entry", name)
		}
	}
}

func hasExactLine(src, want string) bool {
	for _, line := range strings.Split(src, "\n") {
		if strings.TrimSpace(line) == want {
			return true
		}
	}
	return false
}

func skillNames(t *testing.T, root string) map[string]bool {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read skill directory %s: %v", root, err)
	}

	names := make(map[string]bool)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if _, err := os.Stat(filepath.Join(root, name, "SKILL.md")); err != nil {
			t.Errorf("skill %q under %s has no readable SKILL.md: %v", name, root, err)
			continue
		}
		names[name] = true
	}
	return names
}

func repoRelative(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(rel)
}
