SHELL := /bin/bash

BIN := $(PWD)/bin

.PHONY: tools tools-outdated test lint

# Pinned tool versions. Bumped by hand — `make tools-outdated` shows what has moved.
# ponytail: downloads are unverified (no sha256) — the Go checksum database no longer
# covers these. Add <TOOL>_SHA256 vars beside these and check before install.
GOLANGCI_VERSION := v2.13.1

ARCH   := $(shell uname -m | sed -e 's/x86_64/amd64/' -e 's/aarch64/arm64/')
STAMP  := $(BIN)/.versions

# A stamp records a version, not the binary's presence, and .versions is hidden — so
# `rm -rf bin/*` would otherwise leave stamps behind and make `tools` a silent no-op.
$(shell for t in golangci-lint; do \
	[ -e "$(BIN)/$$t" ] || rm -f "$(STAMP)/$$t-"*; done)

tools: $(STAMP)/golangci-lint-$(GOLANGCI_VERSION)

# Each tool's target is a version-stamped marker, not the binary: bumping a version
# var must re-fetch, and a binary path alone cannot express that.
$(STAMP)/golangci-lint-$(GOLANGCI_VERSION):
	@mkdir -p $(BIN) $(STAMP)
	curl -sSfL https://github.com/golangci/golangci-lint/releases/download/$(GOLANGCI_VERSION)/golangci-lint-$(GOLANGCI_VERSION:v%=%)-linux-$(ARCH).tar.gz \
		| tar -xz -C $(BIN) --strip-components=1 golangci-lint-$(GOLANGCI_VERSION:v%=%)-linux-$(ARCH)/golangci-lint
	@rm -f $(STAMP)/golangci-lint-* && touch $@

tools-outdated: ## compare pinned tool versions against upstream latest
	@printf '%-16s %-12s %s\n' TOOL PINNED LATEST
	@for t in "golangci-lint golangci/golangci-lint $(GOLANGCI_VERSION)"; do \
	  set -- $$t; \
	  latest=$$(curl -sSfL https://api.github.com/repos/$$2/releases/latest | jq -r .tag_name); \
	  printf '%-16s %-12s %s\n' "$$1" "$$3" "$$latest"; \
	done

test:
	go test ./...

# go vet is not run separately: golangci-lint's govet linter is on by default (the
# config sets no `default:` key) and enables every analyzer but fieldalignment and
# shadow, so a separate pass re-type-checks every package to add nothing.
lint: ## golangci-lint (matches the CI gate)
	$(BIN)/golangci-lint run ./...
