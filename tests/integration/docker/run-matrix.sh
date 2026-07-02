#!/usr/bin/env bash
set -euo pipefail

required=(docker node)
for bin in "${required[@]}"; do
  if ! command -v "$bin" >/dev/null 2>&1; then
    echo "missing required binary: $bin" >&2
    exit 127
  fi
done

node tests/integration/docker/run-matrix.mjs "$@"
