# mcrcon Makefile. Every target also runs in CI (see .github/workflows/ci.yml).

BINARY  := mcrcon
VERSION ?= dev
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: help build install test vet fmt clean snapshot release-check

help: ## Show this help
	@grep -E '^[a-z-]+: ## ' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ": ## "}; {printf "  %-13s %s\n", $$1, $$2}'

build: ## Build the binary
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BINARY) .

install: ## Install to $GOPATH/bin
	go install -trimpath -ldflags "$(LDFLAGS)" .

test: ## Run tests with the race detector
	go test -race -count=1 ./...

vet: ## Run go vet
	go vet ./...

fmt: ## Format code and report files needing attention
	gofmt -w .
	gofmt -l .

clean: ## Remove build artifacts
	rm -f $(BINARY) mcrcon-test coverage.out

snapshot: ## Local GoReleaser dry run (needs goreleaser installed)
	goreleaser release --snapshot --clean

release-check: ## Validate the GoReleaser config
	goreleaser check
