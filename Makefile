.PHONY: all build build-e2e test test-e2e clean help

# Default target builds both binaries
all: local-review review-e2e

# Build the main review CLI tool (production binary, zero npm dependencies)
local-review: main.go git/*.go server/*.go web/*
	@echo "🔨 Building local-review..."
	go build -o local-review .

build: local-review

# Build the standalone Playwright E2E test runner binary
review-e2e: cmd/review-e2e/main.go e2e/*.go
	@echo "🔨 Building review-e2e test runner..."
	go build -o review-e2e ./cmd/review-e2e

build-e2e: review-e2e

# Run unit tests
test:
	@echo "🧪 Running unit tests..."
	go test ./git ./server -v

# Run Playwright E2E functional tests
test-e2e:
	@echo "🧪 Running Playwright E2E tests..."
	go test -v ./e2e/...

e2e: test-e2e

# Clean built binaries and temporary review output
clean:
	@echo "🧹 Cleaning up..."
	rm -f local-review review-e2e .review.md

help:
	@echo "Local Code Review - Makefile targets:"
	@echo "  make all         - Build both local-review and review-e2e binaries"
	@echo "  make build       - Build local-review binary"
	@echo "  make build-e2e   - Build review-e2e binary"
	@echo "  make test        - Run Go unit tests"
	@echo "  make test-e2e    - Run Go Playwright functional E2E tests"
	@echo "  make clean       - Remove built binaries and temp review files"
