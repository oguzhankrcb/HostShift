#!/usr/bin/env bash
set -euo pipefail

required=(node)
for bin in "${required[@]}"; do
  if ! command -v "$bin" >/dev/null 2>&1; then
    echo "missing required binary: $bin" >&2
    exit 127
  fi
done

if [[ "${HOSTSHIFT_RUN_VM_E2E:-0}" == "1" ]]; then
  provider="${HOSTSHIFT_VM_PROVIDER:-lima}"
  if [[ "$provider" == "lima" ]] && ! command -v limactl >/dev/null 2>&1; then
    echo "missing required binary for VM preflight: limactl" >&2
    exit 127
  fi
fi

node tests/e2e/vm/run-vm-e2e.mjs "$@"
