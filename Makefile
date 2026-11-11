.PHONY: run build install test vet lint audit notices notices-check release-check release-snapshot clean

VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo unversioned)
COMMIT := $(shell git rev-parse HEAD 2>/dev/null)
DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE) -X main.buildSource=source

run:
	go run -ldflags "$(LDFLAGS)" .

build:
	go build -ldflags "$(LDFLAGS)" -o lazyincus .

# Into $GOBIN, ~/go/bin by default; version stamped like `build`.
install:
	go install -ldflags "$(LDFLAGS)" .

test:
	go test ./...

vet:
	go vet ./...

lint:
	golangci-lint run

# Vulnerabilities the code can reach, from the Go vulnerability database.
audit:
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

# THIRD_PARTY_NOTICES.md from the modules the release binaries link, after a
# licence check; CI runs notices-check, which fails when the file is stale.
notices:
	@scripts/notices.sh

notices-check:
	@scripts/notices.sh --check

release-check:
	goreleaser check

release-snapshot:
	goreleaser release --snapshot --clean

clean:
	rm -f lazyincus
	rm -rf dist .archive
