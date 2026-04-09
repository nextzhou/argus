VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

.PHONY: build test lint lint-language lint-ts fmt fix clean

build:
	go build -ldflags "-X main.version=$(VERSION)" -o ./bin/argus ./cmd/argus

test:
	go test ./...

lint: lint-language lint-ts
	golangci-lint run

lint-language:
	bash scripts/check-english-only.sh

lint-ts:
	@if command -v biome >/dev/null 2>&1; then \
		biome check --no-errors-on-unmatched; \
	else \
		echo "biome not found, skipping TypeScript lint"; \
	fi

fmt:
	golangci-lint fmt

fix:
	golangci-lint run --fix

clean:
	rm -rf bin/
