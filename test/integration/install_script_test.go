package integration

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegration_InstallScript_ExistingInstallUpgrade(t *testing.T) {
	script, err := os.ReadFile(filepath.Join("..", "..", "scripts", "install.sh"))
	require.NoError(t, err)

	tests := []struct {
		name        string
		upgradeExit string
		wantCalls   []string
	}{
		{
			name:        "upgrades after refreshing the tap",
			upgradeExit: "0",
			wantCalls: []string{
				"brew list openboot",
				"brew update",
				"brew upgrade openbootdotdev/tap/openboot",
				"openboot version",
				"openboot install --help",
			},
		},
		{
			name:        "reinstalls when upgrade fails",
			upgradeExit: "1",
			wantCalls: []string{
				"brew list openboot",
				"brew update",
				"brew upgrade openbootdotdev/tap/openboot",
				"brew reinstall openbootdotdev/tap/openboot",
				"openboot version",
				"openboot install --help",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			fakeBin := filepath.Join(tmpDir, "bin")
			require.NoError(t, os.Mkdir(fakeBin, 0o755))

			callLog := filepath.Join(tmpDir, "calls.log")
			writeFakeCommand(t, fakeBin, "uname", `#!/bin/bash
case "${1:-}" in
    -s) echo Darwin ;;
    -m) echo arm64 ;;
    *) exit 90 ;;
esac
`)
			writeFakeCommand(t, fakeBin, "xcode-select", "#!/bin/bash\nexit 0\n")
			writeFakeCommand(t, fakeBin, "brew", `#!/bin/bash
printf 'brew %s\n' "$*" >> "$CALL_LOG"
case "${1:-}" in
    list)
        [[ "${2:-}" == "openboot" ]] || exit 91
        exit 0
        ;;
    update) exit 0 ;;
    upgrade) exit "${BREW_UPGRADE_EXIT:-0}" ;;
    reinstall) exit 0 ;;
    *) exit 92 ;;
esac
`)
			writeFakeCommand(t, fakeBin, "openboot", `#!/bin/bash
printf 'openboot %s\n' "$*" >> "$CALL_LOG"
case "${1:-}" in
    version) echo "openboot version v-test" ;;
    install) exit 0 ;;
    *) exit 93 ;;
esac
`)

			// Feed the script through bash's stdin to exercise the documented
			// `curl | bash` execution mode as well as the already-installed path.
			cmd := exec.Command("/bin/bash", "-s", "--", "--help")
			cmd.Stdin = bytes.NewReader(script)
			cmd.Env = []string{
				"PATH=" + fakeBin + ":/usr/bin:/bin",
				"HOME=" + filepath.Join(tmpDir, "home"),
				"CALL_LOG=" + callLog,
				"BREW_UPGRADE_EXIT=" + tt.upgradeExit,
			}

			output, err := cmd.CombinedOutput()
			require.NoError(t, err, "install.sh output:\n%s", output)

			calls, err := os.ReadFile(callLog)
			require.NoError(t, err)
			assert.Equal(t, tt.wantCalls, strings.Split(strings.TrimSpace(string(calls)), "\n"))
			assert.Contains(t, string(output), "OpenBoot is already installed — updating...")
			assert.Contains(t, string(output), "✓ OpenBoot updated!")
			assert.Contains(t, string(output), "openboot version v-test")
			assert.NotContains(t, string(output), "Reinstall?")
			assert.NotContains(t, string(output), "Installing OpenBoot via Homebrew...")
		})
	}
}

func writeFakeCommand(t *testing.T, dir, name, contents string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o755))
}
