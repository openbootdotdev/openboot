package brew

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func captureOutput(t *testing.T, fn func()) string {
	t.Helper()

	oldStdout := os.Stdout
	oldStderr := os.Stderr

	r, w, err := os.Pipe()
	require.NoError(t, err)

	os.Stdout = w
	os.Stderr = w
	defer func() {
		os.Stdout = oldStdout
		os.Stderr = oldStderr
	}()

	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()

	fn()

	require.NoError(t, w.Close())

	return <-done
}

func TestInstallWithProgress_BatchesAliasResolutionAndSkipsCaskAliases(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "brew-calls.log")

	setupFakeBrew(t, fmt.Sprintf(`#!/bin/sh
log_file=%q
printf '%%s\n' "$*" >> "$log_file"

if [ "$1" = "list" ] && [ "$2" = "--formula" ]; then
  exit 0
fi
if [ "$1" = "list" ] && [ "$2" = "--cask" ]; then
  exit 0
fi
if [ "$1" = "info" ]; then
  shift 2
  for arg in "$@"; do
    if [ "$arg" = "firefox" ]; then
      echo "unexpected cask alias resolution" >&2
      exit 1
    fi
  done
  cat <<'EOF'
[{"name":"postgresql@16"},{"name":"kubernetes-cli"}]
EOF
  exit 0
fi
if [ "$1" = "update" ]; then
  exit 0
fi
if [ "$1" = "install" ] && [ "$2" = "--cask" ]; then
  exit 0
fi
if [ "$1" = "install" ]; then
  exit 0
fi
exit 0
`, logPath))

	originalCheckNetwork := checkNetworkFunc
	checkNetworkFunc = func() error { return nil }
	t.Cleanup(func() { checkNetworkFunc = originalCheckNetwork })

	formulae, casks, err := InstallWithProgress([]string{"postgresql", "kubectl"}, []string{"firefox"}, false)
	require.NoError(t, err)
	assert.Equal(t, []string{"postgresql@16", "kubernetes-cli"}, formulae)
	assert.Equal(t, []string{"firefox"}, casks)

	logContent, err := os.ReadFile(logPath)
	require.NoError(t, err)

	var infoLines []string
	for _, line := range strings.Split(strings.TrimSpace(string(logContent)), "\n") {
		if strings.HasPrefix(line, "info --json") {
			infoLines = append(infoLines, line)
		}
	}

	require.Len(t, infoLines, 1)
	assert.Equal(t, "info --json postgresql kubectl", infoLines[0])
	assert.NotContains(t, infoLines[0], "firefox")
}

func TestInstallWithProgress_RetrySuccessTracksCanonicalNames(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "brew-calls.log")
	statePath := filepath.Join(tmpDir, "foo-attempts")

	setupFakeBrew(t, fmt.Sprintf(`#!/bin/sh
log_file=%q
state_file=%q
printf '%%s\n' "$*" >> "$log_file"

if [ "$1" = "list" ] && [ "$2" = "--formula" ]; then
  exit 0
fi
if [ "$1" = "list" ] && [ "$2" = "--cask" ]; then
  exit 0
fi
if [ "$1" = "info" ]; then
  cat <<'EOF'
[{"name":"foo-canonical"}]
EOF
  exit 0
fi
if [ "$1" = "update" ]; then
  exit 0
fi
if [ "$1" = "install" ] && [ "$2" = "foo" ]; then
  attempt=0
  if [ -f "$state_file" ]; then
    attempt=$(cat "$state_file")
  fi
  attempt=$((attempt + 1))
  echo "$attempt" > "$state_file"
  if [ "$attempt" -eq 1 ]; then
    echo "Error: Connection timed out while downloading"
    exit 1
  fi
  exit 0
fi
if [ "$1" = "install" ] && [ "$2" = "--cask" ]; then
  exit 0
fi
if [ "$1" = "install" ]; then
  exit 0
fi
exit 0
`, logPath, statePath))

	originalCheckNetwork := checkNetworkFunc
	originalSleep := sleepFunc
	checkNetworkFunc = func() error { return nil }
	sleepFunc = func(time.Duration) {}
	t.Cleanup(func() {
		checkNetworkFunc = originalCheckNetwork
		sleepFunc = originalSleep
	})

	var formulae, casks []string
	var err error
	output := captureOutput(t, func() {
		formulae, casks, err = InstallWithProgress([]string{"foo"}, nil, false)
	})

	require.NoError(t, err)
	assert.Equal(t, []string{"foo-canonical"}, formulae)
	assert.Empty(t, casks)
	assert.Contains(t, output, "retry succeeded")
	assert.NotContains(t, output, "packages failed to install")

	logContent, err := os.ReadFile(logPath)
	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(string(logContent), "info --json foo"))
	assert.Equal(t, 2, strings.Count(string(logContent), "install foo"))
}
