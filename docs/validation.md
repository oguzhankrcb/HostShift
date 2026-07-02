# Validation and Release Gates

HostShift is allowed to manage risky target-side operations only because the source side is deliberately constrained. A release is not ready until these gates pass with the current code and the current generated binary.

## Required Local Gates

Run these before publishing a release candidate:

```bash
make test
make build
make test-integration-docker
HOSTSHIFT_RUN_DOCKER_MATRIX=1 make test-integration-docker
make test-e2e-vm
HOSTSHIFT_RUN_VM_E2E=1 bash tests/e2e/vm/run-vm-e2e.sh --apply
make release-snapshot
```

`make test-integration-docker` and `make test-e2e-vm` are safe dry-run checks by default. The environment variables opt into real Docker or VM execution and are intentionally explicit.

VM command timeouts are controlled with `HOSTSHIFT_VM_COMMAND_TIMEOUT_MS`, `HOSTSHIFT_VM_LIMACTL_TIMEOUT_MS`, `HOSTSHIFT_VM_HOSTSHIFT_TIMEOUT_MS`, and `HOSTSHIFT_VM_SSH_TIMEOUT_MS`. These fail stuck provider, SSH, or HostShift phases explicitly instead of leaving a silent release gate.

GitHub hosted macOS runners can run Lima preflight but cannot reliably boot nested Lima VMs. The real VM apply gate must run locally or on a self-hosted macOS runner labeled `hostshift-vm` through `.github/workflows/vm-e2e-apply.yml` before tagging a release.

## Source Safety Gate

Every real migration-style test must prove the source remained unchanged:

- source commands must be allowlisted fact reads or typed read-only exports
- no source command may use `sudo`, package managers, service managers, firewall managers, file writes, key changes, snapshots, or maintenance mode
- Docker matrix tests must verify source fixture checksums before and after apply paths
- VM e2e tests must capture and compare a source checksum snapshot around HostShift execution

If a workload cannot be read consistently online without mutating the source, HostShift must report a blocker instead of silently skipping it.

## Matrix Gate

The release matrix must include at least:

- Ubuntu to Ubuntu
- Ubuntu to Debian
- Debian to Ubuntu
- Debian to Debian

The current Docker and VM matrices cover:

- `ubuntu22 -> ubuntu22, ubuntu24, ubuntu25, debian12`
- `debian12 -> ubuntu22, ubuntu24, ubuntu25, debian12, debian13`

## Real VM Gate

Docker is the fast workload loop, but it cannot prove all platform behavior. Before a release, the VM apply gate must verify:

- package installation on fresh provider-like images
- systemd unit state
- Nginx config validation and HTTP health
- MySQL/MariaDB row and checksum parity
- PostgreSQL row and checksum parity
- UFW and nftables rule state
- target boot persistence after restart
- source checksum immutability

## Artifact Gate

Release artifacts must include:

- checksummed binaries covered by a keyless-signed checksum file
- `checksums.txt`
- `checksums.txt.sig`
- `checksums.txt.pem`
- SPDX SBOM
- GitHub artifact provenance attestation for release artifacts
- archived README, license, security policy, docs, and examples

`make release-snapshot` is the local artifact smoke test. Tagged GitHub releases use `.github/workflows/release.yml`.

## GitHub Release Gate

Tagged releases must not publish until these workflow jobs pass:

- `quick-gates`: Node tests, Go tests, build, Docker dry-run, VM dry-run, and release snapshot
- `docker-matrix`: real Docker matrix with `HOSTSHIFT_RUN_DOCKER_MATRIX=1`
- `vm-e2e-preflight`: hosted macOS Lima provider preflight
- `release`: GoReleaser packaging, keyless checksum signing, and artifact provenance attestation

Manual `workflow_dispatch` runs produce a snapshot package after the same hosted gates. Tag pushes produce the public GitHub release. The release checklist still requires a separate successful local or self-hosted `VM E2E Apply` run for the real VM gate.
