SHELL := /bin/bash

BIN := $(PWD)/bin

.PHONY: tools tools-outdated test lint setup-hooks scan-secrets vuln sonar

# Pinned tool versions. Bumped by hand — `make tools-outdated` shows what has moved.
# ponytail: golangci-lint download is unverified (no sha256) — the Go checksum
# database doesn't cover it. gitleaks/lefthook are sha256-verified below, one digest
# per ARCH. govulncheck has no release binary; `go install` gets sumdb verification
# for free instead of a manual digest.
GOLANGCI_VERSION    := v2.13.1
GITLEAKS_VERSION    := v8.30.1
LEFTHOOK_VERSION    := v2.1.14
GOVULNCHECK_VERSION := v1.8.0

GITLEAKS_SHA256_amd64 := 551f6fc83ea457d62a0d98237cbad105af8d557003051f41f3e7ca7b3f2470eb
GITLEAKS_SHA256_arm64 := e4a487ee7ccd7d3a7f7ec08657610aa3606637dab924210b3aee62570fb4b080
LEFTHOOK_SHA256_amd64 := 2be3187101b3a2edd5edf8b546aa2e3820531c517b4cd0216601a9c06f07c42d
LEFTHOOK_SHA256_arm64 := 47736ec15b894e29a3d3a8e7b550dd2ae47416dcc588371794783735608edf50

ARCH   := $(shell uname -m | sed -e 's/x86_64/amd64/' -e 's/aarch64/arm64/')
STAMP  := $(BIN)/.versions

# A stamp records a version, not the binary's presence, and .versions is hidden — so
# `rm -rf bin/*` would otherwise leave stamps behind and make `tools` a silent no-op.
$(shell for t in golangci-lint gitleaks lefthook govulncheck; do \
	[ -e "$(BIN)/$$t" ] || rm -f "$(STAMP)/$$t-"*; done)

tools: $(STAMP)/golangci-lint-$(GOLANGCI_VERSION) $(STAMP)/gitleaks-$(GITLEAKS_VERSION) \
       $(STAMP)/lefthook-$(LEFTHOOK_VERSION) $(STAMP)/govulncheck-$(GOVULNCHECK_VERSION)

# Each tool's target is a version-stamped marker, not the binary: bumping a version
# var must re-fetch, and a binary path alone cannot express that.
$(STAMP)/golangci-lint-$(GOLANGCI_VERSION):
	@mkdir -p $(BIN) $(STAMP)
	curl -sSfL https://github.com/golangci/golangci-lint/releases/download/$(GOLANGCI_VERSION)/golangci-lint-$(GOLANGCI_VERSION:v%=%)-linux-$(ARCH).tar.gz \
		| tar -xz -C $(BIN) --strip-components=1 golangci-lint-$(GOLANGCI_VERSION:v%=%)-linux-$(ARCH)/golangci-lint
	@rm -f $(STAMP)/golangci-lint-* && touch $@

$(STAMP)/gitleaks-$(GITLEAKS_VERSION):
	@mkdir -p $(BIN) $(STAMP)
	curl -sSfL -o /tmp/gitleaks.tar.gz https://github.com/gitleaks/gitleaks/releases/download/$(GITLEAKS_VERSION)/gitleaks_$(GITLEAKS_VERSION:v%=%)_linux_$(ARCH:amd64=x64).tar.gz
	echo "$(GITLEAKS_SHA256_$(ARCH))  /tmp/gitleaks.tar.gz" | sha256sum -c -
	tar -xz -C $(BIN) -f /tmp/gitleaks.tar.gz gitleaks
	@rm -f /tmp/gitleaks.tar.gz $(STAMP)/gitleaks-* && touch $@

$(STAMP)/lefthook-$(LEFTHOOK_VERSION):
	@mkdir -p $(BIN) $(STAMP)
	curl -sSfL -o /tmp/lefthook.gz https://github.com/evilmartians/lefthook/releases/download/$(LEFTHOOK_VERSION)/lefthook_$(LEFTHOOK_VERSION:v%=%)_Linux_$(ARCH:amd64=x86_64).gz
	echo "$(LEFTHOOK_SHA256_$(ARCH))  /tmp/lefthook.gz" | sha256sum -c -
	gunzip -c /tmp/lefthook.gz > $(BIN)/lefthook
	chmod +x $(BIN)/lefthook
	@rm -f /tmp/lefthook.gz $(STAMP)/lefthook-* && touch $@

# no release binary; the module proxy + sumdb verify it, so no manual digest.
$(STAMP)/govulncheck-$(GOVULNCHECK_VERSION):
	@mkdir -p $(BIN) $(STAMP)
	GOBIN=$(BIN) go install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)
	@rm -f $(STAMP)/govulncheck-* && touch $@

tools-outdated: ## compare pinned tool versions against upstream latest
	@printf '%-16s %-12s %s\n' TOOL PINNED LATEST
	@for t in "golangci-lint golangci/golangci-lint $(GOLANGCI_VERSION)" \
	          "gitleaks gitleaks/gitleaks $(GITLEAKS_VERSION)" \
	          "lefthook evilmartians/lefthook $(LEFTHOOK_VERSION)"; do \
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

setup-hooks: ## install lefthook's git hooks into .git
	$(BIN)/lefthook install

scan-secrets: ## gitleaks over full history, as CI/pre-push do
	$(BIN)/gitleaks git --redact --no-banner .

vuln: ## govulncheck against the module
	$(BIN)/govulncheck ./...

sonar: ## opt-in SonarQube scan, on demand only — start SonarQube first: docker compose --profile sonar up -d sonarqube (in devstack)
	./scripts/sonar-scan.sh
