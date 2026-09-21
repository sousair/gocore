#!/usr/bin/env bash
# Opt-in SonarQube scan. The dind sidecar can't see /workspace, so sources are
# tar-piped into a named docker volume instead of bind-mounted. Start SonarQube
# first: docker compose --profile sonar up -d sonarqube (in devstack).
set -euo pipefail

: "${SONAR_TOKEN:?SONAR_TOKEN is not set — export the SonarQube token first}"

REPO_ROOT="$(git -C "$(dirname "${BASH_SOURCE[0]}")/.." rev-parse --show-toplevel)"
IMAGE="sonarsource/sonar-scanner-cli@sha256:a3f4215076706c95a17a68c19322ee916e40a3acd081a8c1a1e839e0194afa57"
VOLUME="gocore-sonar-src"

docker volume rm -f "$VOLUME" >/dev/null 2>&1 || true
docker volume create "$VOLUME" >/dev/null
tar -cf - --exclude-vcs -C "$REPO_ROOT" . \
	| docker run --rm -i -v "$VOLUME":/usr/src busybox tar xf - -C /usr/src

docker run --rm --network devstack_default --cpus=3 \
	-v "$VOLUME":/usr/src -w /usr/src \
	-e SONAR_TOKEN -e SONAR_HOST_URL=http://sonarqube:9000 \
	"$IMAGE" -Dsonar.scanner.javaOpts=-Xmx2g

docker volume rm -f "$VOLUME" >/dev/null 2>&1 || true
