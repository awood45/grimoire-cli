BINARY_NAME := grimoire-cli
BINARY_DIR := bin
CMD_DIR := ./cmd/grimoire-cli
COVERAGE_THRESHOLD := 95
COVERAGE_FILE := coverage.out
FILTERED_COVERAGE_FILE := coverage-filtered.out

.PHONY: build test test-coverage test-coverage-detail lint fmt fmt-check check audit setup-hooks clean install vet

## Build

build: ## Build the binary
	@mkdir -p $(BINARY_DIR)
	go build -o $(BINARY_DIR)/$(BINARY_NAME) $(CMD_DIR)

install: ## Install binary to $GOPATH/bin
	go install $(CMD_DIR)

## Testing

test: ## Run tests
	go test ./...

test-coverage: ## Run tests with coverage enforcement (95% threshold)
	go test -race -coverprofile=$(COVERAGE_FILE) -covermode=atomic ./...
	@# Filter out test infrastructure (fakes, testutil) from aggregate coverage.
	@grep -v '/testing/' $(COVERAGE_FILE) | grep -v '/testutil/' > $(FILTERED_COVERAGE_FILE) || true
	@if ! head -1 $(FILTERED_COVERAGE_FILE) | grep -q '^mode:'; then \
		(head -1 $(COVERAGE_FILE); cat $(FILTERED_COVERAGE_FILE)) > $(FILTERED_COVERAGE_FILE).tmp; \
		mv $(FILTERED_COVERAGE_FILE).tmp $(FILTERED_COVERAGE_FILE); \
	fi
	@echo "--- Coverage Report (production code only) ---"
	@go tool cover -func=$(FILTERED_COVERAGE_FILE)
	@COVERAGE=$$(go tool cover -func=$(FILTERED_COVERAGE_FILE) | grep total | awk '{print $$3}' | sed 's/%//'); \
	if [ -z "$$COVERAGE" ]; then \
		echo "No test coverage data. Write tests to meet the $(COVERAGE_THRESHOLD)% threshold."; \
	elif [ $$(echo "$$COVERAGE < $(COVERAGE_THRESHOLD)" | bc -l) -eq 1 ]; then \
		echo "FAIL: Coverage $$COVERAGE% is below $(COVERAGE_THRESHOLD)% threshold"; \
		rm -f $(FILTERED_COVERAGE_FILE); \
		exit 1; \
	else \
		echo "OK: Coverage $$COVERAGE% meets $(COVERAGE_THRESHOLD)% threshold"; \
	fi
	@rm -f $(FILTERED_COVERAGE_FILE)

test-coverage-detail: ## Show per-package coverage breakdown
	@go test -coverprofile=$(COVERAGE_FILE) -covermode=atomic ./... 2>/dev/null
	@echo "--- Per-Package Coverage ---"
	@go test -cover ./... 2>/dev/null | grep -v "no test files" | \
		awk '{gsub(/.*\/internal\//, ""); gsub(/\t/, " "); print}' | \
		column -t 2>/dev/null || true
	@echo ""
	@COVERAGE=$$(go tool cover -func=$(COVERAGE_FILE) | grep total | awk '{print $$3}'); \
	echo "Aggregate: $$COVERAGE"

test-coverage-html: test-coverage ## Open coverage report in browser
	go tool cover -html=$(COVERAGE_FILE) -o coverage.html
	open coverage.html

## Code Quality

lint: ## Run linter
	golangci-lint run ./...

fmt: ## Auto-format code
	gofmt -w .
	goimports -w .

fmt-check: ## Check formatting (no changes)
	@test -z "$$(gofmt -l .)" || (echo "Files need formatting:"; gofmt -l .; exit 1)
	@test -z "$$(goimports -l .)" || (echo "Files need import fixing:"; goimports -l .; exit 1)

vet: ## Run go vet
	go vet ./...

## Security

audit: ## Run dependency vulnerability scan
	govulncheck ./...

## Combined

check: fmt-check lint vet test-coverage audit ## Run all checks (CI equivalent)
	@echo "All checks passed."

## Setup

setup-hooks: ## Install pre-commit hooks
	@cp hooks/pre-commit .git/hooks/pre-commit
	@chmod +x .git/hooks/pre-commit
	@echo "Pre-commit hook installed."

## Utilities

clean: ## Remove build artifacts
	rm -rf $(BINARY_DIR) $(COVERAGE_FILE) $(FILTERED_COVERAGE_FILE) coverage.html

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help
