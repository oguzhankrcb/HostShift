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

## Codex Plugin

HostShift also ships as a Codex plugin from the same repository. Install the CLI first, then add the repo marketplace:

```bash
codex plugin marketplace add https://github.com/oguzhankrcb/HostShift.git
codex plugin add hostshift@hostshift
```

For local plugin development from a checkout:

```bash
codex plugin marketplace add .
codex plugin add hostshift@hostshift
```

The plugin is an assistant layer around the CLI. It gives Codex the `migrate-server` workflow and source read-only rules; the migration itself still runs through `hostshift`.

Start a new Codex thread after installing or updating the plugin so Codex loads the bundled skill.

## MCP And Claude

HostShift includes a stdio MCP server for AI clients that support MCP:

```bash
hostshift mcp stdio
```

Claude Desktop can use `integrations/claude/claude_desktop_config.example.json` as a starting point. The MCP server exposes planning and dry-run tools only; `--apply` remains a human-operated CLI action.

Validate the MCP and Claude integration surface:

```bash
hostshift mcp doctor --json
```
