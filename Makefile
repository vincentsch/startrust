VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
# Release tags start with v; the version inside the binary does not.
BUILD_VERSION := $(patsubst v%,%,$(VERSION))
LDFLAGS := -s -w -X github.com/vincentsch/startrust/internal/version.Version=$(BUILD_VERSION) -X github.com/vincentsch/startrust/internal/version.Commit=$(COMMIT)
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64
NATIVE_SUFFIX := $(if $(filter windows,$(shell go env GOOS)),.exe)

.PHONY: build build-all test race lint live clickhouse conformance docs docs-check links clean

build:
	mkdir -p bin
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/startrust$(NATIVE_SUFFIX) ./cmd/startrust

build-all:
	mkdir -p dist
	rm -f dist/startrust-* dist/checksums.txt
	@set -e; for platform in $(PLATFORMS); do \
		os=$${platform%/*}; arch=$${platform#*/}; \
		asset=startrust-$$os-$$arch; \
		if [ "$$os" = windows ]; then asset=$$asset.exe; fi; \
		echo "Building $$asset"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -ldflags "$(LDFLAGS)" -o "dist/$$asset" ./cmd/startrust; \
	done
	@cd dist && if command -v sha256sum >/dev/null 2>&1; then \
		sha256sum startrust-* > checksums.txt; \
	else shasum -a 256 startrust-* > checksums.txt; fi

test:
	go test ./...

race:
	go test -race ./...

lint:
	@test -z "$$(gofmt -l cmd internal scripts)" || { gofmt -l cmd internal scripts; exit 1; }
	go vet ./...

# Talks to the real GitHub API and GH Archive. Uses GITHUB_TOKEN when set.
live:
	STARTRUST_LIVE=1 go test ./internal/cli -run Live -count=1 -v

# Runs the GH Archive query on a real ClickHouse server in Docker.
clickhouse:
	./scripts/clickhouse-test.sh

conformance:
	./scripts/conformance.sh

docs:
	go run ./cmd/generate-command-docs

docs-check:
	go run ./cmd/generate-command-docs --check

links:
	python3 scripts/docs-links.py

clean:
	rm -f bin/startrust bin/startrust.exe dist/startrust-* dist/checksums.txt
