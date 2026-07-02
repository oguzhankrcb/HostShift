# Release Process

HostShift releases are allowed only after the source-read-only invariant has been tested in fast, Docker, and VM environments.

## Local Candidate

Run these commands from a clean working tree:

```bash
make test
make build
HOSTSHIFT_RUN_DOCKER_MATRIX=1 make test-integration-docker
HOSTSHIFT_RUN_VM_E2E=1 bash tests/e2e/vm/run-vm-e2e.sh --apply
make release-snapshot
git status --short
```

The final `git status --short` must be empty except for intentionally ignored generated artifacts such as `dist/` and `.cache/`.

## GitHub Candidate

Use the CI workflow manually before tagging. The hosted run executes quick gates, the real Docker matrix, and hosted macOS Lima preflight.

GitHub hosted macOS does not reliably support booting nested Lima VMs. Run the real VM apply gate locally or through the `VM E2E Apply` workflow on a self-hosted macOS runner labeled `hostshift-vm`. Keep that runner offline by default; see `docs/self-hosted-runner.md`.

```bash
HOSTSHIFT_RUN_VM_E2E=1 bash tests/e2e/vm/run-vm-e2e.sh --apply
```

Only tag a release after both the hosted CI candidate and the real VM apply gate pass.

## Public Release

Create an annotated tag after the manual candidate succeeds:

```bash
git tag -a v0.3.0 -m "HostShift v0.3.0"
git push origin v0.3.0
```

The tag-triggered Release workflow publishes GoReleaser artifacts only after:

- quick unit and build gates pass
- the real Docker migration matrix passes
- hosted Lima preflight passes
- a separate local or self-hosted Lima VM apply matrix has passed
- release artifacts are checksummed, keyless-signed, and attested

Do not publish a release if any source immutability check, checksum check, database parity check, firewall check, systemd check, or post-reboot verification fails.

Each tag release uploads `checksums.txt`, `checksums.txt.sig`, and `checksums.txt.pem`. Verify the signed checksum file before trusting a downloaded binary:

```bash
cosign verify-blob \
  --certificate checksums.txt.pem \
  --signature checksums.txt.sig \
  --certificate-identity-regexp 'https://github.com/.*/.github/workflows/release.yml@refs/tags/v.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  checksums.txt
shasum -a 256 -c checksums.txt
```
