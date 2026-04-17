.PHONY: all build test cover vet clean

GO ?= go
PKGS := ./...

all: test build

build:
	$(GO) build -o bin/ana ./cmd/ana

vet:
	$(GO) vet $(PKGS)

test:
	$(GO) test -race $(PKGS)

# cover runs -race -coverprofile across ALL packages (so the race detector
# still sees main), then gates the coverage total against ./internal/... only.
# cmd/ana is intentionally excluded from the gate: it is pure wiring whose
# uncovered lines (os.Exit, signal.NotifyContext, the main() entry itself)
# cannot be reached from a go-test process without exec'ing a subprocess.
cover:
	$(GO) test -race -coverprofile=c.out $(PKGS)
	$(GO) test -race -coverprofile=c.internal.out ./internal/...
	$(GO) tool cover -func=c.internal.out | tail -1
	@$(GO) tool cover -func=c.internal.out | awk '/^total:/ { split($$3, a, "%"); if (a[1]+0 < 100) { print "internal coverage below 100% ("$$3")"; exit 1 } }'

clean:
	rm -rf bin c.out c.internal.out
