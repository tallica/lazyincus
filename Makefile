.PHONY: run build test vet lint audit release-check release-snapshot clean

VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo unversioned)
COMMIT := $(shell git rev-parse HEAD 2>/dev/null)
DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE) -X main.buildSource=source

run:
	go run -ldflags "$(LDFLAGS)" .

build:
	go build -ldflags "$(LDFLAGS)" -o lazyincus .

test:
	go test ./...

vet:
	go vet ./...

lint:
	golangci-lint run

# Vulnerabilities the code can reach, from the Go vulnerability database.
audit:
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

release-check:
	goreleaser check

release-snapshot:
	goreleaser release --snapshot --clean

clean:
	rm -f lazyincus
	rm -rf dist .archive
