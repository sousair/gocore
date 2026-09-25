#!/usr/bin/env sh
# Resolves a hook tool: the repo's pinned copy in bin/ (from `make tools`) first,
# then the same binary on PATH — the jail image ships gitleaks and govulncheck at
# the pinned versions — so a fresh worktree can commit before `make tools` runs.
# Exits non-zero when neither exists; callers decide whether that blocks.
set -eu
name="$1"
if [ -x "bin/$name" ]; then
	echo "bin/$name"
	exit 0
fi
if path="$(command -v "$name")"; then
	echo "$path"
	exit 0
fi
echo "$name not found in bin/ or on PATH — run 'make tools'" >&2
exit 1
