.PHONY: help all build test cover vet lint deps release-local clean e2e e2e-sweep e2e-dryrun

GO ?= go
PKGS := ./...
STATICCHECK_VERSION := 2025.1.1

# help prints each target's leading `##` doc comment. Keep target docs on the
# same line as the target for awk to find them.
help: ## Show this help.
	@awk 'BEGIN {FS = ":.*## "; printf "Targets:\n"} /^[a-zA-Z0-9_.-]+:.*## / {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

all: test build ## Run tests then build the binary.

build: ## Build ./bin/ana.
	$(GO) build -o bin/ana ./cmd/ana

vet: ## Run go vet.
	$(GO) vet $(PKGS)

# deps installs development tools pinned for reproducibility.
deps: ## Install pinned dev tools (staticcheck).
	$(GO) install honnef.co/go/tools/cmd/staticcheck@$(STATICCHECK_VERSION)

# lint mirrors the CI lint job: gofmt check, go vet, staticcheck. Assumes
# `make deps` has been run at least once so staticcheck is on PATH.
lint: ## Run gofmt / go vet / staticcheck (mirrors CI).
	@if [ -n "$$(gofmt -l .)" ]; then \
		echo "Code is not formatted. Run 'go fmt ./...'"; \
		gofmt -d .; \
		exit 1; \
	fi
	$(GO) vet $(PKGS)
	staticcheck $(PKGS)

# release-local validates .goreleaser.yml and produces a throwaway snapshot
# in dist/. Requires goreleaser on PATH (brew install goreleaser or
# https://goreleaser.com/install/). No publish, no tag push.
release-local: ## Validate goreleaser config + build a snapshot (no publish).
	goreleaser check
	goreleaser release --snapshot --clean

test: ## Run the race-enabled test suite.
	$(GO) test -race $(PKGS)

# cover runs -race -coverprofile across ALL packages (so the race detector
# still sees main), then gates the coverage total against ./internal/... only.
# cmd/ana is intentionally excluded from the gate: it is pure wiring whose
# uncovered lines (os.Exit, signal.NotifyContext, the main() entry itself)
# cannot be reached from a go-test process without exec'ing a subprocess.
cover: ## Run tests with coverage; fail if ./internal/... drops below 100%.
	$(GO) test -race -coverprofile=c.out $(PKGS)
	$(GO) test -race -coverprofile=c.internal.out ./internal/...
	$(GO) tool cover -func=c.internal.out | tail -1
	@$(GO) tool cover -func=c.internal.out | awk '/^total:/ { split($$3, a, "%"); if (a[1]+0 < 100) { print "internal coverage below 100% ("$$3")"; exit 1 } }'

clean: ## Remove build + coverage artifacts.
	rm -rf bin dist c.out c.internal.out

ENV_FILE := .env
# LOAD_ENV sources the repo-root .env if present, exporting each KEY=VAL so
# the spawned `go test` process inherits ANA_E2E_ENDPOINT / ANA_E2E_TOKEN /
# ANA_E2E_EXPECT_ORG_ID. No-op when .env is absent. Must stay inline (a
# single shell line) because each make-recipe line spawns its own shell.
LOAD_ENV := if [ -f $(ENV_FILE) ]; then set -a; . ./$(ENV_FILE); set +a; fi;

# e2e runs the live smoke suite against app.textql.com. Requires env:
#   ANA_E2E_ENDPOINT   (e.g. https://app.textql.com)
#   ANA_E2E_TOKEN      API key for the demo org
#   ANA_E2E_EXPECT_ORG human-readable org name the token must resolve to
# -p 1 serializes packages so parallel resource names cannot collide; the
# suite uses disposable-parent-nesting to stay auto-revertable.
e2e: ## Run live e2e tests against app.textql.com (requires ANA_E2E_* env).
	@$(LOAD_ENV) $(GO) test -p 1 -count=1 -timeout 10m ./e2e/...

# e2e-dryrun lists every planned mutation without hitting the network. The
# harness short-circuits Run() and returns empty results; see harness/harness.go.
e2e-dryrun: ## List planned e2e mutations without hitting the network.
	@$(LOAD_ENV) ANA_E2E_DRYRUN=1 $(GO) test -p 1 -count=1 -timeout 2m -v ./e2e/...

# e2e-sweep cleans any anacli-e2e-* leftovers from prior crashed runs. Useful
# to run manually before a fresh suite if a previous run died mid-test.
e2e-sweep: ## Clean up anacli-e2e-* leftovers from prior crashed runs.
	@$(LOAD_ENV) ANA_E2E_SWEEP_ONLY=1 $(GO) test -p 1 -count=1 -timeout 2m ./e2e/...
