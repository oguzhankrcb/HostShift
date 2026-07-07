#!/usr/bin/env bash
set -euo pipefail

required=(docker)
for bin in "${required[@]}"; do
  if ! command -v "$bin" >/dev/null 2>&1; then
    echo "missing required binary: $bin" >&2
    exit 127
  fi
done

if [[ -x "./dist/hostshift" ]] && ./dist/hostshift help | grep -q "docker-e2e"; then
  exec ./dist/hostshift docker-e2e "$@"
fi

if ! command -v go >/dev/null 2>&1; then
  echo "missing required binary: go" >&2
  exit 127
fi

mkdir -p .cache/go-build .cache/go-mod
export GOCACHE="${GOCACHE:-$PWD/.cache/go-build}"
export GOMODCACHE="${GOMODCACHE:-$PWD/.cache/go-mod}"
exec go run ./cmd/hostshift docker-e2e "$@"
