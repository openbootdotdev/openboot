.PHONY: test-unit test-integration test-e2e test-coverage test-all

BINARY_NAME=openboot
BINARY_PATH=./$(BINARY_NAME)
COVERAGE_FILE=coverage.out
COVERAGE_HTML=coverage.html

test-unit:
	go test -v ./...

test-integration:
	go test -v -tags=integration ./...

test-e2e: build
	go test -v -tags=e2e -short ./...

test-coverage:
	go test -v -coverprofile=$(COVERAGE_FILE) ./...
	go tool cover -html=$(COVERAGE_FILE) -o $(COVERAGE_HTML)
	@echo "Coverage report generated: $(COVERAGE_HTML)"

test-all:
	@echo "Running all tests..."
	$(MAKE) test-unit
	$(MAKE) test-integration
	-$(MAKE) test-e2e
	$(MAKE) test-coverage

build:
	go build -o $(BINARY_PATH) ./cmd/openboot

clean:
	rm -f $(BINARY_PATH) $(COVERAGE_FILE) $(COVERAGE_HTML)
