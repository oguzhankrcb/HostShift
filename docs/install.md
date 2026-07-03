# Install HostShift

HostShift is distributed as a signed GitHub release archive. Build from source during development, but prefer release binaries for migration rehearsals and production use.

## From Release

Download the archive for your platform from the GitHub release page:

- `hostshift_<version>_linux_amd64.tar.gz`
- `hostshift_<version>_linux_arm64.tar.gz`
- `hostshift_<version>_darwin_amd64.tar.gz`
- `hostshift_<version>_darwin_arm64.tar.gz`

Verify the signed checksum file before installing:

```bash
cosign verify-blob \
  --certificate checksums.txt.pem \
  --signature checksums.txt.sig \
  --certificate-identity-regexp 'https://github.com/.*/.github/workflows/release.yml@refs/tags/v.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  checksums.txt

shasum -a 256 -c checksums.txt
```

Install the binary:

```bash
tar xzf hostshift_<version>_<os>_<arch>.tar.gz
install -m 0755 hostshift /usr/local/bin/hostshift
hostshift version
```

Use a user-writable directory such as `~/.local/bin` when you do not want to install with elevated privileges.

## From Source

```bash
git clone https://github.com/oguzhankrcb/HostShift.git
cd HostShift
make build
./dist/hostshift version
```

## Codex Plugin

The Codex plugin is distributed from the same repository through a repo marketplace. Install the CLI first, then add the marketplace:

```bash
codex plugin marketplace add https://github.com/oguzhankrcb/HostShift.git
codex plugin add hostshift@hostshift
```

For local plugin development from a checkout:

```bash
codex plugin marketplace add .
codex plugin add hostshift@hostshift
```

The plugin is an assistant layer around the CLI. It helps Codex follow HostShift's reviewed migration workflow and source read-only policy; the actual migration actions still run through `hostshift`.

After installing or updating the plugin, start a new Codex thread before testing `migrate-server`.

## First Rehearsal

Start with dry-run commands. `prepare`, `sync`, and `verify` do not execute target mutations unless `--apply` is passed.

```bash
hostshift doctor --source old-server --target new-server --json
hostshift discover --source old-server --name rehearsal --profile rehearsal.profile.yaml --json
hostshift plan --profile rehearsal.profile.yaml --target new-server --json
hostshift prepare --profile rehearsal.profile.yaml --target new-server --json
hostshift sync --profile rehearsal.profile.yaml --target new-server --json
hostshift verify --profile rehearsal.profile.yaml --target new-server --json
```

Review all blockers and planned actions before running any `--apply` command. HostShift never writes to the source server; target-side apply commands can still install packages, write files, load container images, restore databases, and adjust target-only configuration.
