---
title: Test Matrix
description: Docker and VM validation architecture for HostShift migrations.
---

HostShift uses two integration layers because Docker and real VMs catch different failure classes.

## Fast Local Gates

```bash
npm --prefix docs-site ci
npm run docs:build
npm run docs:compose:config
make test-go
make build
make test-integration-docker
make test-e2e-vm
make release-snapshot
```

The default Docker and VM commands are dry-run or scaffold validation paths unless their `HOSTSHIFT_RUN_*` environment variables are set.

## Docker Matrix

Location:

```text
tests/integration/docker
```

Run dry validation:

```bash
make test-integration-docker
```

Run real Docker matrix:

```bash
HOSTSHIFT_RUN_DOCKER_MATRIX=1 make test-integration-docker
```

The matrix creates SSH-managed source and target containers and runs HostShift as a real CLI against them.

Covered fixture behavior:

- Docker Compose app fixture
- standalone container metadata
- file-set transfer
- MySQL restore
- PostgreSQL restore
- HTTP health check
- Laravel-style database connectivity check
- source immutability marker checks
- cross-distro matrix selection

Useful diagnostics:

```bash
hostshift matrix docker --list
hostshift matrix docker --list-images
hostshift matrix docker --pair 'ubuntu22->debian12' --json
make docker-pull-fixtures
HOSTSHIFT_DOCKER_PULL_TIMEOUT_MS=60000 HOSTSHIFT_RUN_DOCKER_MATRIX=1 make test-integration-docker
```

## VM Matrix

Location:

```text
tests/e2e/vm
```

Run dry validation:

```bash
make test-e2e-vm
```

Run provider preflight:

```bash
HOSTSHIFT_RUN_VM_E2E=1 make test-e2e-vm
```

Run real apply matrix:

```bash
make build
HOSTSHIFT_RUN_VM_E2E=1 bash tests/e2e/vm/run-vm-e2e.sh --apply
```

Run one pair:

```bash
HOSTSHIFT_RUN_VM_E2E=1 bash tests/e2e/vm/run-vm-e2e.sh --pair 'ubuntu22->debian12' --apply
```

Useful diagnostics:

```bash
hostshift matrix vm --list
hostshift matrix vm --pair 'ubuntu22->debian12' --json
```

The VM runner uses Lima. It validates package installation, systemd service state, firewall state, boot persistence, HTTP health, MySQL parity, PostgreSQL parity, and source snapshot immutability against booted VMs.

## Current VM Matrix

Initial matrix shape:

- `ubuntu22 -> ubuntu22`
- `ubuntu22 -> ubuntu24`
- `ubuntu22 -> ubuntu25`
- `ubuntu22 -> debian12`
- `debian12 -> ubuntu22`
- `debian12 -> ubuntu24`
- `debian12 -> ubuntu25`
- `debian12 -> debian12`
- `debian12 -> debian13`

## GitHub Actions

`CI` runs quick hosted gates on every push and pull request.

Scheduled or manually dispatched runs can execute the Docker matrix. The real VM apply gate runs only on an explicitly labeled self-hosted macOS runner:

```text
self-hosted, macOS, hostshift-vm
```

This prevents public repository workflows from landing heavy VM workloads on the developer machine unless a runner is deliberately started.
