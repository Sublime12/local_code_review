.PHONY: all build build-e2e test test-e2e clean help

all: review review-e2e

review: main.go git/*.go server/*.go web/*
	go build -o review .

build: review

review-e2e: e2e/*.go git/*.go server/*.go web/*
	go test -c -o review-e2e ./e2e

build-e2e: review-e2e

test:
	go test . ./git ./server -v

test-e2e: review-e2e
	./review-e2e -test.v

e2e: test-e2e

clean:
	rm -f review review-e2e .review.md

help:
	@echo "Local Code Review - Makefile targets:"
	@echo "  make all         - Build both local-review and review-e2e binaries"
	@echo "  make build       - Build local-review binary"
	@echo "  make build-e2e   - Build review-e2e binary"
	@echo "  make test        - Run Go unit tests"
	@echo "  make test-e2e    - Run Go Playwright functional E2E tests"
	@echo "  make clean       - Remove built binaries and temp review files"
