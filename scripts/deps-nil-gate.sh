#!/usr/bin/env bash
# Fails when the diff against the merge base adds a non-test `.deps.X == nil` or
# `.deps.X != nil` line (ADR _global:decisions/code-shape-001, decision 2): every
# Dependencies field is required, so a nil guard means a dependency was left
# optional. Existing guards are not touched; only added lines count.
#
# Allowlist: scripts/deps-nil-allowlist.txt, one fixed string per line, matched
# against "path: code" of each offending line. Blank lines and # comments ignored.
set -euo pipefail

base_ref="${DEPS_NIL_BASE:-origin/master}"
allowlist="scripts/deps-nil-allowlist.txt"

# Resolve the base before the pipeline: its `|| true` would turn a missing ref
# (shallow clone, unfetched origin) into an empty diff and a silent pass.
if ! git rev-parse --verify --quiet "$base_ref^{commit}" >/dev/null; then
	echo "deps-nil gate: base ref $base_ref not found; fetch it (CI needs fetch-depth: 0)" >&2
	exit 1
fi

added="$(git diff --merge-base "$base_ref" -U0 -- '*.go' ':!*_test.go' |
	awk '/^\+\+\+ b\// { file = substr($0, 7); next }
	     /^\+/ && !/^\+\+\+/ { print file ": " substr($0, 2) }' |
	grep -E '\.deps\.[A-Za-z]+ (!=|==) nil' || true)"

if [ -n "$added" ] && [ -f "$allowlist" ]; then
	patterns="$(grep -vE '^[[:space:]]*(#|$)' "$allowlist" || true)"
	if [ -n "$patterns" ]; then
		added="$(grep -vF -- "$patterns" <<<"$added" || true)"
	fi
fi

if [ -n "$added" ]; then
	echo "deps-nil gate: new nil guard on a Dependencies field (every field is required; use a no-op implementation or a testDeps builder):" >&2
	echo "$added" >&2
	exit 1
fi
