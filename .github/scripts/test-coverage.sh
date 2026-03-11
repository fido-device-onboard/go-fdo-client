#!/usr/bin/env bash
set -xeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
PROJECT="go-fdo-client"

COVERDIR="${COVERDIR:-${REPO_ROOT}/test/coverage}"
GOCOVERDIR="${GOCOVERDIR:-${COVERDIR}/integration}"
COMPOSE_FILE="${REPO_ROOT}/.github/compose/servers.yaml"

cleanup() {
	docker compose -f "${COMPOSE_FILE}" logs || true
	docker compose -f "${COMPOSE_FILE}" down || true
}

# Clean and prepare coverage directories
rm -rf "${COVERDIR}"
mkdir -p "${COVERDIR}/unit" "${GOCOVERDIR}" "${COVERDIR}/merged"

# Unit tests with coverage
GOCOVERDIR="${COVERDIR}/unit" go test -coverpkg=./... -covermode=atomic ./...

# Build coverage-instrumented binary
make -C "${REPO_ROOT}" build GOFLAGS="-cover -covermode=atomic"
mkdir -p "${REPO_ROOT}/bin"
install -m 755 "${REPO_ROOT}/${PROJECT}" "${REPO_ROOT}/bin/${PROJECT}"
export PATH="${REPO_ROOT}/bin:${PATH}"

# Integration tests
export GOCOVERDIR
# Note: fdo-utils.sh sets -xeuo pipefail which matches our settings
. "${SCRIPT_DIR}/fdo-utils.sh"
generate_certs
trap cleanup EXIT
docker compose -f "${COMPOSE_FILE}" up -d --build
test_onboarding

# Merge and generate coverage report
go tool covdata merge -i="${COVERDIR}/unit,${GOCOVERDIR}" -o="${COVERDIR}/merged"
go tool covdata textfmt -i="${COVERDIR}/merged" -o="${COVERDIR}/coverage.out"
go tool cover -html="${COVERDIR}/coverage.out" -o "${COVERDIR}/coverage.html"
