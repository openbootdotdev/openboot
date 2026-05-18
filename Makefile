.PHONY: test-unit test-e2e test-coverage test-all \
       test-vm test-vm-run test-vm-parallel test-vm-inner test-vm-inner-run \
       install-hooks uninstall-hooks

# VM A: install/journey tests that touch real system state (longest-running).
VM_A_TESTS := TestVM_Journey_FirstTimeUser|TestVM_Journey_DryRunIsCompletelySafe|TestVM_Journey_FullSetupConfiguresEverything|TestVM_Interactive_InstallScript
# VM B: all other VM tests — dotfiles, macOS, edge cases, smoke, real-install, sync.
VM_B_TESTS := TestVM_Journey_Dotfiles|TestVM_Journey_MacOS|TestVM_Edge_|TestSmoke_|TestE2E_

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

test-e2e: build
	go test -v -tags=e2e -short ./...

test-coverage:
	go test -v -timeout 5m -coverprofile=$(COVERAGE_FILE) ./...
	go tool cover -html=$(COVERAGE_FILE) -o $(COVERAGE_HTML)
	@echo "Coverage report generated: $(COVERAGE_HTML)"

test-all:
	@echo "Running all tests..."
	$(MAKE) test-unit
	$(MAKE) test-coverage

# =============================================================================
# Tart VM e2e — destructive tests run inside a throwaway Tart VM provisioned
# by scripts/vm/run.sh. Files tagged `e2e,vm` run via `make test-vm-inner`;
# files tagged `e2e && !vm` (auth, snapshot_api) run as L3 on the host.
#
# Requires Apple Silicon + Tart installed locally. See scripts/vm/README.md
# for one-time setup. The relevant targets are defined immediately below
# this header: test-vm, test-vm-run, test-vm-inner, test-vm-inner-run.
# =============================================================================

# Developer-facing: provisions a Tart VM and runs the full e2e suite inside.
test-vm: build
	scripts/vm/run.sh test-vm-inner

# Developer-facing: runs one named test inside a Tart VM.
test-vm-run: build
	scripts/vm/run.sh "test-vm-inner-run TEST=$(TEST)"

# Developer-facing: runs e2e in two parallel VMs — VM A (system tests) and
# VM B (mock-server tests). Requires ~16 GB RAM and 8 cores free.
# Exit code is non-zero if either VM fails.
test-vm-parallel: build
	@OPENBOOT_VM_TEST='$(VM_A_TESTS)' scripts/vm/run.sh test-vm-inner & PID_A=$$!; \
	OPENBOOT_VM_TEST='$(VM_B_TESTS)' scripts/vm/run.sh test-vm-inner & PID_B=$$!; \
	A_EXIT=0; B_EXIT=0; \
	wait $$PID_A || A_EXIT=$$?; \
	wait $$PID_B || B_EXIT=$$?; \
	[ $$A_EXIT -eq 0 ] && [ $$B_EXIT -eq 0 ]

# In-VM: invoked over SSH by run.sh — not called by developers directly.
test-vm-inner:
	go test -v -timeout 60m -tags="e2e,vm" ./test/e2e/...

test-vm-inner-run:
	go test -v -timeout 45m -tags="e2e,vm" -run '$(TEST)' ./test/e2e/...

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
# pre-push:   go test ./...       (~75s L1 unit + integration + contract, runs on every push)
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
