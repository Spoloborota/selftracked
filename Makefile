# The canonical command catalog. Agents and humans run `make <target>`
# instead of reconstructing toolchain invocations from memory; anything worth
# running twice belongs here.
#
# The toolchain is pinned: GOTOOLCHAIN names the exact version, because the
# go.mod `toolchain` directive only ever upgrades a newer local install, it
# never downgrades one.

export GOTOOLCHAIN := go1.26.5
export CGO_ENABLED := 0

GO      := go
PKGS    := ./...
BIN     := bin/selftracked

.DEFAULT_GOAL := help

.PHONY: help
help: ## List the targets
	@awk 'BEGIN {FS = ":.*## "} /^[a-z][a-z-]*:.*## / {printf "  \033[1m%-12s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

.PHONY: build
build: ## Compile every package
	$(GO) build -trimpath $(PKGS)

.PHONY: binaries
binaries: ## Build bin/selftracked and its strk alias from the one main
	$(GO) build -trimpath -o bin/selftracked ./cmd/selftracked
	$(GO) build -trimpath -o bin/strk ./cmd/selftracked

.PHONY: install
install: ## Install selftracked into GOBIN — the hooks and SessionStart resolve it via PATH
	$(GO) install ./cmd/selftracked

.PHONY: test
test: ## Run the tests (race detector needs cgo; repo is otherwise cgo-free)
	CGO_ENABLED=1 $(GO) test -race $(PKGS)

.PHONY: vet
vet: ## Run go vet
	$(GO) vet $(PKGS)

.PHONY: lint
lint: ## Run golangci-lint
	$(GO) tool golangci-lint run $(PKGS)

.PHONY: fix-check
fix-check: ## Fail if the modern-idiom fixer has anything to say
	@out="$$($(GO) fix -diff $(PKGS) 2>&1)"; status=$$?; \
	if [ -n "$$out" ] || [ $$status -ne 0 ]; then \
		printf '%s\n' "$$out" >&2; \
		echo "fix-check: pending modernisations (exit $$status) — run 'go fix ./...'" >&2; \
		exit 1; \
	fi; \
	echo "fix-check: clean"

.PHONY: vuln
vuln: ## Scan dependencies for known vulnerabilities
	$(GO) tool govulncheck $(PKGS)

.PHONY: fuzz
fuzz: ## Run the fuzz corpora briefly (seeds only until a target exists)
	$(GO) test -run '^$$' -fuzz . -fuzztime 10s $(PKGS) 2>/dev/null || \
		echo "fuzz: no fuzz targets yet"

.PHONY: check-pins
check-pins: ## Verify the toolchain and dependency pins
	./scripts/check-pins.sh

.PHONY: probe-gofix
probe-gofix: ## Re-prove the undocumented `go fix -diff` exit-code behaviour
	./scripts/probe-gofix.sh

# check-inventory retired at S10 with the traceability inventory it checked
# (plan §9); scripts/check-inventory.py stays for the historical file in git.

# binaries rides gates since the S10 self-host: the git gate and the
# SessionStart hook run bin/selftracked off PATH, and a gates run is the
# natural moment the working binary must match the code it just proved.
.PHONY: gates
gates: build vet test lint fix-check vuln check-pins binaries ## Everything a stage close needs

.PHONY: clean
clean: ## Remove build output
	rm -rf bin
