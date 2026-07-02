---
title: Install
description: Install HostShift from a signed release or build from source.
---

HostShift is distributed as a signed GitHub release archive. Build from source during development; use release binaries for migration rehearsals and production operations.

## Release Binary

Download the archive for your platform:

- `hostshift_<version>_linux_amd64.tar.gz`
- `hostshift_<version>_linux_arm64.tar.gz`
- `hostshift_<version>_darwin_amd64.tar.gz`
- `hostshift_<version>_darwin_arm64.tar.gz`

Verify the signed checksum file:

```bash
cosign verify-blob \
  --certificate checksums.txt.pem \
  --signature checksums.txt.sig \
  --certificate-identity-regexp 'https://github.com/.*/.github/workflows/release.yml@refs/tags/v.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  checksums.txt

shasum -a 256 -c checksums.txt
```

Install:

```bash
tar xzf hostshift_<version>_<os>_<arch>.tar.gz
install -m 0755 hostshift /usr/local/bin/hostshift
hostshift version
```

Use `~/.local/bin` when you do not want elevated install privileges.

## Build From Source

```bash
git clone https://github.com/oguzhankrcb/HostShift.git
cd HostShift
make build
./dist/hostshift version
```
