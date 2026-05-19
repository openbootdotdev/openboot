.PHONY: test-unit test-e2e test-coverage test-all \
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
