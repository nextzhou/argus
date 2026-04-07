VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

.PHONY: build test lint fmt clean

build:
	go build -ldflags "-X main.version=$(VERSION)" -o ./bin/argus ./cmd/argus

test:
	go test ./...

lint:
	golangci-lint run

fmt:
	goimports -w .

clean:
	rm -rf bin/
