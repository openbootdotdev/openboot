#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PASS=0
FAIL=0

green() { printf "\033[32m%s\033[0m\n" "$1"; }
red() { printf "\033[31m%s\033[0m\n" "$1"; }
yellow() { printf "\033[33m%s\033[0m\n" "$1"; }

pass() { ((PASS++)); green "  [PASS] $1"; }
fail() { ((FAIL++)); red "  [FAIL] $1"; }

echo ""
echo "OpenBoot Test Suite"
echo "==================="
echo ""

echo "1. Shell Syntax Validation"
echo "--------------------------"

for script in boot.sh install.sh lib/*.sh lib/**/*.sh; do
    if [[ -f "$SCRIPT_DIR/$script" ]]; then
        if bash -n "$SCRIPT_DIR/$script" 2>/dev/null; then
            pass "$script"
        else
            fail "$script - syntax error"
        fi
    fi
done
echo ""

echo "2. Package Definitions"
echo "----------------------"

source "$SCRIPT_DIR/lib/packages.sh"

PRESETS=(minimal standard full devops frontend data mobile ai)

for preset in "${PRESETS[@]}"; do
    cli_packages=$(get_packages "$preset" "cli" 2>/dev/null)
    cask_packages=$(get_packages "$preset" "cask" 2>/dev/null)
    
    cli_count=$(echo "$cli_packages" | wc -w | tr -d ' ')
    cask_count=$(echo "$cask_packages" | wc -w | tr -d ' ')
    
    if [[ $cli_count -gt 0 && $cask_count -gt 0 ]]; then
        pass "$preset: $cli_count CLI, $cask_count cask packages"
    else
        fail "$preset: missing packages (CLI: $cli_count, cask: $cask_count)"
    fi
done
echo ""

echo "3. Preset Descriptions"
echo "----------------------"

for preset in "${PRESETS[@]}"; do
    desc=$(get_preset_description "$preset")
    if [[ -n "$desc" && "$desc" != "Unknown preset" ]]; then
        pass "$preset: $desc"
    else
        fail "$preset: missing description"
    fi
done
echo ""

echo "4. Dry Run Tests"
echo "----------------"

boot_output=$("$SCRIPT_DIR/boot.sh" --dry-run 2>&1 || true)
if echo "$boot_output" | grep -q "Dry Run"; then
    pass "boot.sh --dry-run"
else
    fail "boot.sh --dry-run"
fi

install_output=$("$SCRIPT_DIR/install.sh" --preset minimal --dry-run 2>&1 || true)
if echo "$install_output" | grep -q "DRY-RUN"; then
    pass "install.sh --preset minimal --dry-run"
else
    fail "install.sh --preset minimal --dry-run"
fi
echo ""

echo "5. Help Output"
echo "--------------"

if "$SCRIPT_DIR/boot.sh" --help 2>/dev/null | grep -q "OpenBoot"; then
    pass "boot.sh --help"
else
    fail "boot.sh --help"
fi

if "$SCRIPT_DIR/install.sh" --help 2>/dev/null | grep -q "OpenBoot"; then
    pass "install.sh --help"
else
    fail "install.sh --help"
fi
echo ""

echo "==================="
echo "Results: $PASS passed, $FAIL failed"
echo ""

if [[ $FAIL -gt 0 ]]; then
    red "Some tests failed!"
    exit 1
else
    green "All tests passed!"
    exit 0
fi
