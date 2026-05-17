.PHONY: test-unit test-e2e test-coverage test-all \
       test-vm test-vm-run test-vm-inner test-vm-inner-run \
       install-hooks uninstall-hooks

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
# by scripts/vm/run.sh. The 12 files in test/e2e/ run via the `e2e,vm` build
# tag; the VM driver SSHs in and invokes `make test-vm-inner`.
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

# In-VM: invoked over SSH by run.sh — not called by developers directly.
test-vm-inner:
	go test -v -timeout 60m -tags="e2e,vm" ./test/e2e/...

test-vm-inner-run:
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
