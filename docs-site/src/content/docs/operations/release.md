---
title: Release Process
description: Release checklist and public tag workflow.
---

HostShift releases are allowed only after the source-read-only invariant has been tested in fast, Docker, and VM environments.

## Pre-Tag Checklist

- hosted CI manual candidate passed on `main`
- self-hosted `VM E2E Apply` passed on the `hostshift-vm` runner or an equivalent local VM apply run passed
- `make release-snapshot` passed locally
- `dist/checksums.txt` and `dist/hostshift.sbom.spdx.json` were produced
- `git status --short` is clean
- `CHANGELOG.md` has an entry for the version being tagged

## Create A Tag

```bash
git tag -a v0.3.0 -m "HostShift v0.3.0"
git push origin v0.3.0
```

The tag-triggered Release workflow publishes GoReleaser artifacts only after quick gates, Docker matrix, and hosted Lima preflight pass. The real VM apply gate is run separately on local or self-hosted macOS hardware.

## Verify Checksums

```bash
cosign verify-blob \
  --certificate checksums.txt.pem \
  --signature checksums.txt.sig \
  --certificate-identity-regexp 'https://github.com/.*/.github/workflows/release.yml@refs/tags/v.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  checksums.txt

shasum -a 256 -c checksums.txt
```
