---
title: Validation Gates
description: Test gates required before release.
---

HostShift release candidates must pass more than unit tests.

## Fast Gates

```bash
make test-go
make build
make test-integration-docker
make test-e2e-vm
make release-snapshot
```

## Docker Matrix

Run the real SSH-driven Docker matrix:

```bash
HOSTSHIFT_RUN_DOCKER_MATRIX=1 make test-integration-docker
```

The matrix verifies cross-distro migrations, source immutability markers, file copy, MySQL restore, PostgreSQL restore, HTTP health, Laravel-style DB connectivity, and checksum assertions.

## VM Apply Gate

Run the real Lima VM apply matrix locally or on the self-hosted `hostshift-vm` macOS runner:

```bash
HOSTSHIFT_RUN_VM_E2E=1 bash tests/e2e/vm/run-vm-e2e.sh --apply
```

The VM gate covers package installation, systemd behavior, firewall state, boot persistence, database parity, HTTP health, and source checksum immutability.

## Release Artifacts

`make release-snapshot` must produce:

- `dist/hostshift`
- `dist/checksums.txt`
- `dist/hostshift.sbom.spdx.json`

Tagged releases also upload signed checksum artifacts and GitHub artifact provenance attestations.
