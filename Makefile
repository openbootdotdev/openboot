.PHONY: test-unit test-integration test-e2e test-destructive test-smoke test-smoke-prebuilt test-coverage test-all \
       test-vm test-vm-short test-vm-run test-vm-quick test-vm-release test-vm-full

BINARY_NAME=openboot
BINARY_PATH=./$(BINARY_NAME)
VERSION ?= dev
LDFLAGS=-X github.com/openbootdotdev/openboot/internal/cli.version=$(VERSION)
COVERAGE_FILE=coverage.out
COVERAGE_HTML=coverage.html

test-unit:
	go test -v -timeout 5m ./...

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
	-$(MAKE) test-e2e
	$(MAKE) test-coverage

# =============================================================================
# VM-based E2E tests (Tart VMs) — three levels
# =============================================================================

# L1: Quick validation (~5min) — run after code changes
#     Uses -short flag: skips tests that install real packages
test-vm-quick: build
	go test -v -timeout 10m -tags="e2e,vm" -short ./test/e2e/...

# L2: Release validation (~20min) — run before tagging a release
#     Core user journeys: dry-run safety, install + verify, diff/clean cycle,
#     manual uninstall recovery, full setup, error messages
test-vm-release: build
	go test -v -timeout 30m -tags="e2e,vm" \
	  -run "TestVM_Infra|TestVM_Journey_DryRun|TestVM_Journey_FirstTimeUser|TestVM_Journey_ManualUninstall|TestVM_Journey_DiffConsistency|TestVM_Journey_FullSetup|TestVM_Journey_ErrorMessages" \
	  ./test/e2e/...

# L3: Full validation (~60min) — run for major releases or CI
#     All 48 tests: journeys + edge cases + commands + interactive
test-vm-full: build
	go test -v -timeout 90m -tags="e2e,vm" ./test/e2e/...

# Aliases
test-vm: test-vm-release
test-vm-short: test-vm-quick

# Single VM test by name (e.g. make test-vm-run TEST=TestVM_Journey_DryRun)
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
