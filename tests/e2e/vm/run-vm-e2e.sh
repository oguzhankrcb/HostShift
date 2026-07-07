#!/usr/bin/env bash
set -euo pipefail

if [[ "${HOSTSHIFT_RUN_VM_E2E:-0}" == "1" ]]; then
  provider="${HOSTSHIFT_VM_PROVIDER:-lima}"
  if [[ "$provider" == "lima" ]] && ! command -v limactl >/dev/null 2>&1; then
    echo "missing required binary for VM preflight: limactl" >&2
    exit 127
  fi
fi

if [[ -x "./dist/hostshift" ]] && ./dist/hostshift help | grep -q "vm-e2e"; then
  exec ./dist/hostshift vm-e2e "$@"
fi

if ! command -v go >/dev/null 2>&1; then
  echo "missing required binary: go" >&2
  exit 127
fi

mkdir -p .cache/go-build .cache/go-mod
export GOCACHE="${GOCACHE:-$PWD/.cache/go-build}"
export GOMODCACHE="${GOMODCACHE:-$PWD/.cache/go-mod}"
exec go run ./cmd/hostshift vm-e2e "$@"
