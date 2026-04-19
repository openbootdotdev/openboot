.PHONY: test-unit test-integration test-e2e test-destructive test-smoke test-smoke-prebuilt test-coverage test-all \
       test-vm test-vm-run test-vm-quick test-vm-release test-vm-full \
       install-hooks uninstall-hooks \
       quality-lint quality-security

BINARY_NAME=openboot
BINARY_PATH=./$(BINARY_NAME)
VERSION ?= dev
LDFLAGS=-X github.com/openbootdotdev/openboot/internal/cli.version=$(VERSION)
COVERAGE_FILE=coverage.out
COVERAGE_HTML=coverage.html

test-unit:
	go test -v -race -timeout 5m ./...

lint:
	golangci-lint run ./...

test-integration:
	go test -v -timeout 5m -tags=integration ./...

test-e2e: build
	go test -v -tags=e2e -short ./...

test-destructive: build
	go test -v -timeout 15m -tags="e2e,destructive" ./...

test-smoke: build
	go test -v -timeout 20m -tags="e2e,destructive,smoke" -run TestSmoke ./...

# test-smoke-prebuilt: like test-smoke but skips build (uses pre-built binary in PATH or ./openboot)
test-smoke-prebuilt:
	go test -v -timeout 20m -tags="e2e,destructive,smoke" -run TestSmoke ./...

test-coverage:
	go test -v -timeout 5m -coverprofile=$(COVERAGE_FILE) ./...
	go tool cover -html=$(COVERAGE_FILE) -o $(COVERAGE_HTML)
	@echo "Coverage report generated: $(COVERAGE_HTML)"

test-all:
	@echo "Running all tests..."
	$(MAKE) test-unit
	$(MAKE) test-integration
	$(MAKE) test-coverage

# =============================================================================
# Destructive macOS E2E tests — three levels
# =============================================================================
#
# These tests install real packages and modify ~/.zshrc / macOS defaults on
# the host they run on. They are intended for ephemeral macOS CI runners
# (GitHub Actions macos-latest) or a throwaway VM.
#
# On a developer machine `go test -tags="e2e,vm"` will skip unless you set
# OPENBOOT_E2E_DESTRUCTIVE=1 (see testutil/machost.go). Don't set that
# unless you mean it.

# L1: Quick sanity (~1min) — host/arch checks only, no package installs
test-vm-quick: build
	go test -v -timeout 5m -tags="e2e,vm" -run "TestVM_Infra" ./test/e2e/...

# L2: Release validation (~20min) — core user journeys
test-vm-release: build
	go test -v -timeout 30m -tags="e2e,vm" \
	  -run "TestVM_Infra|TestVM_Journey_DryRunIsCompletelySafe|TestVM_Journey_FirstTimeUser|TestVM_Journey_FullSetupConfiguresEverything|TestE2E_DryRunMinimal|TestE2E_SnapshotCapture" \
	  ./test/e2e/...

# L3: Full validation (~60min) — everything under -tags="e2e,vm"
test-vm-full: build
	go test -v -timeout 90m -tags="e2e,vm" ./test/e2e/...

# Aliases
test-vm: test-vm-release

# Single test by name (e.g. make test-vm-run TEST=TestVM_Journey_DryRunIsCompletelySafe)
test-vm-run: build
	go test -v -timeout 45m -tags="e2e,vm" -run $(TEST) ./test/e2e/...

build:
	go build -ldflags="$(LDFLAGS)" -o $(BINARY_PATH) ./cmd/openboot

build-release:
	@echo "Building optimized release binary (version=$(VERSION))..."
	go build -ldflags="-s -w $(LDFLAGS)" -trimpath -o $(BINARY_PATH) ./cmd/openboot
	@echo "Original size: $$(du -h $(BINARY_PATH) | cut -f1)"
	@if command -v upx >/dev/null 2>&1; then \
		echo "Compressing with UPX..."; \
		upx --best --lzma --force-macos $(BINARY_PATH); \
		echo "Compressed size: $$(du -h $(BINARY_PATH) | cut -f1)"; \
	else \
		echo "UPX not found. Install with: brew install upx"; \
	fi

clean:
	rm -f $(BINARY_PATH) $(COVERAGE_FILE) $(COVERAGE_HTML)

# =============================================================================
# Git hooks — symlink scripts/hooks/ into .git/hooks/
# =============================================================================
#
# After cloning or pulling new hook scripts, run:
#   make install-hooks
#
# pre-commit: go vet + go build   (<5s, runs on every commit)
# pre-push:   go test ./...       (~15s L1 unit+contract, runs on every push)
#
# Skip once:  git commit --no-verify  |  git push --no-verify

install-hooks:
	@mkdir -p .git/hooks
	@for hook in pre-commit pre-push; do \
		ln -sf ../../scripts/hooks/$$hook .git/hooks/$$hook; \
		echo "✓ installed .git/hooks/$$hook -> scripts/hooks/$$hook"; \
	done

uninstall-hooks:
	@for hook in pre-commit pre-push; do \
		rm -f .git/hooks/$$hook; \
		echo "✓ removed .git/hooks/$$hook"; \
	done

# =============================================================================
# Code quality
# =============================================================================

quality-lint: ## Run golangci-lint
	golangci-lint run ./...

quality-security: ## Run gosec
	gosec ./...
