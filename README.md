# HostShift

Move Ubuntu and Debian web servers without touching the source.

HostShift discovers a live server over SSH, builds a reviewable migration profile, prepares a clean target, streams data across, and verifies the result. It enforces a strictly read-only source policy: no source-side writes, no `sudo`, no service restarts, no package installs, no firewall changes, no keys added, no snapshots, no maintenance mode.

[Install](#install) | [Codex Plugin](#codex-plugin) | [What You Get](#what-you-get) | [How It Works](#how-it-works) | [Validation](#validation) | [Safety Model](#safety-model)

---

HostShift is a CLI-first migration tool and Codex plugin for practical Linux web-stack moves. It is built for the annoying real case: you have working production services on one Ubuntu or Debian host, you need to recreate them somewhere else, and stopping or modifying the source would be unacceptable.

The migration engine is deterministic Go code. The Codex plugin is an operator layer that helps Codex follow the workflow, keep the source read-only contract, and run the right `hostshift` commands in the right order.

## Before / After

Manual server move:

> Inspect services by hand, copy configs, guess package names, dump databases, chase missing firewall rules, forget an SSH setting, retry under pressure, and hope the old server still behaves.

HostShift move:

```bash
hostshift discover --source old-server --name rehearsal --profile rehearsal.profile.yaml --json
hostshift plan --profile rehearsal.profile.yaml --target new-server --json
hostshift prepare --profile rehearsal.profile.yaml --target new-server --json
hostshift sync --profile rehearsal.profile.yaml --target new-server --json
hostshift verify --profile rehearsal.profile.yaml --target new-server --json
```

Same goal. Less guessing. Source stays read-only.

```text
source mutation policy       blocked
target mutations             planned and reviewable
transfer path                streamed over SSH
profile format               YAML + JSON Schema
verification                 workload checks, not vibes
resume/audit                 state + JSONL journal
```

## Install

Use release binaries for migration rehearsals and production work. Full release verification lives in [docs/install.md](./docs/install.md).

```bash
tar xzf hostshift_<version>_<os>_<arch>.tar.gz
install -m 0755 hostshift /usr/local/bin/hostshift
hostshift version
```

Build from source during development:

```bash
git clone https://github.com/oguzhankrcb/HostShift.git
cd HostShift
make build
./dist/hostshift version
```

Run directly from source:

```bash
go run ./cmd/hostshift version
go run ./cmd/hostshift doctor --source old-server --target new-server --json
go run ./cmd/hostshift discover --source old-server --name example --profile example.profile.yaml --json
go run ./cmd/hostshift plan --profile examples/profile.yaml --target new-server --json
go run ./cmd/hostshift prepare --profile examples/profile.yaml --target new-server --json
go run ./cmd/hostshift sync --profile examples/profile.yaml --target new-server --json
go run ./cmd/hostshift verify --profile examples/profile.yaml --target new-server --json
go run ./cmd/hostshift profile migrate --input examples/profile.yaml --output /tmp/profile.v2.json
```

With an installed binary, replace `go run ./cmd/hostshift` with `hostshift`.

> [!IMPORTANT]
> `prepare`, `sync`, and `verify` default to dry-run mode and write resumable state. Add `--apply` only after reviewing blockers, actions, streams, and rollback metadata.

## Codex Plugin

HostShift ships as a Codex plugin under [plugins/hostshift](./plugins/hostshift). The plugin does not replace the CLI. It gives Codex the migration workflow, safety rules, and operator guidance while the deterministic `hostshift` binary performs discovery, planning, transfer, and verification.

Install from this repo marketplace:

```bash
codex plugin marketplace add https://github.com/oguzhankrcb/HostShift.git
codex plugin add hostshift@hostshift
```

For local development from a checkout:

```bash
codex plugin marketplace add .
codex plugin add hostshift@hostshift
```

Start a new Codex thread after installing or updating the plugin so the bundled `migrate-server` skill is loaded.

## Supported Platforms

The first support matrix focuses on current Ubuntu and Debian server targets.

| Family | Supported releases |
| --- | --- |
| Ubuntu | 22.04 LTS, 24.04 LTS, 25.10 interim, 26.04 LTS |
| Debian | 12, 13 |

EOL targets are blocked by default. EOL sources can still be read when the migration remains source-safe. Source and target versions do not need to match; HostShift checks platform capabilities instead of assuming identical images.

## What You Get

| Area | What HostShift does |
| --- | --- |
| Source discovery | Reads OS, packages, services, Docker, web server, SSH, firewall, and database facts through allowlisted commands. |
| Profiles | Writes v2 YAML profiles with JSON Schema validation, workload metadata, checks, target settings, and secret env references. |
| Planning | Emits reviewable `Action{id, phase, hostRole, impact, command, preconditions, rollback}` records before apply. |
| Target prepare | Plans target-only package installs and config writes for Docker, databases, Nginx/Apache, SSH, UFW, nftables, and app dependencies. |
| Transfers | Streams files, container images, MySQL/MariaDB, and PostgreSQL from source stdout to target stdin. |
| Verification | Checks HTTP, Laravel database connectivity, files, DB scalar queries, systemd services, firewall rules, nftables, and Nginx config. |
| State | Keeps resumable run state and JSONL audit logs. |
| Safety | Blocks source-side mutation attempts in code and tests. |

See [examples/profile.v2.yaml](./examples/profile.v2.yaml) and [examples/web-stack-v2.yaml](./examples/web-stack-v2.yaml) for real profile shapes.

## Workloads

Current workload coverage includes:

- Docker Compose projects
- standalone Docker containers
- bind-mounted file sets
- streamed Docker images
- MySQL and MariaDB
- PostgreSQL
- Nginx and Apache configuration
- systemd service checks
- SSH server settings
- UFW and nftables firewall rules
- Laravel-style database connectivity checks

Redis is intentionally blocked unless an existing snapshot or replica can be read without modifying the source. Docker named volumes are blockers until the profile gives an explicit safe strategy.

## How It Works

1. **Doctor** checks connectivity, platform support, and obvious blockers.
2. **Discover** reads source facts through allowlisted, read-only operations.
3. **Profile** records workloads, target config, checks, and env-var secret references.
4. **Plan** turns the profile into reviewable actions and streams.
5. **Prepare** applies target-only package and config changes when approved.
6. **Sync** streams data from source stdout into target-side writes.
7. **Verify** runs target-side checks and records audit output.
8. **Resume/status** continue interrupted runs from state.

Sync plans may include streams such as `tar --create -> tar --extract`, `docker image save -> docker image load`, `mysqldump -> mysql`, and `pg_dump -> pg_restore`. The source side produces stdout; the target side mutates only the target.

## Validation

Quick checks:

```bash
make test
make test-go
make build
npm run docs:build
npm run docs:compose:config
```

Docker migration matrix:

```bash
make test-integration-docker
HOSTSHIFT_RUN_DOCKER_MATRIX=1 make test-integration-docker
```

Real VM e2e:

```bash
make test-e2e-vm
HOSTSHIFT_RUN_VM_E2E=1 make test-e2e-vm
make build
HOSTSHIFT_RUN_VM_E2E=1 bash tests/e2e/vm/run-vm-e2e.sh --pair ubuntu22->debian12 --apply
```

The Docker matrix runs SSH-driven source/target containers. The VM e2e layer boots real Lima instances, captures source snapshots, runs `discover`, `plan`, `prepare`, `sync`, and `verify` over SSH, validates workload parity, reboots the target, verifies persistence, and checks that the source snapshot did not change.

Release candidates must satisfy the gates in [docs/validation.md](./docs/validation.md), including source immutability, cross-distro Docker coverage, real VM apply coverage, checksums, and SBOM output.

## Safety Model

HostShift's core invariant is strict:

> The source server is an immutable observation endpoint.

Allowed on source:

- read inventory
- inspect configuration
- stream typed exports to stdout
- run allowlisted fact commands

Forbidden on source:

- `sudo`
- writing files
- installing packages
- restarting or stopping services
- changing firewall rules
- adding SSH keys
- creating snapshots
- placing apps into maintenance mode
- creating database dump files on disk

Known limits are reported instead of hidden. Live filesystem streaming is not a point-in-time snapshot. MySQL dumps can acquire metadata locks. PostgreSQL dumps are consistent per selected database, not across unrelated databases. Workloads that cannot be read safely are blockers, not silent skips.

## Documentation

The documentation website is built with Astro Starlight under [docs-site](./docs-site).

```bash
cd docs-site
npm install
npm run dev
```

Or run it with Docker Compose:

```bash
docker compose -f docs-site/compose.yml up --build
```

Open `http://localhost:4321`.

Useful docs:

- [Install](./docs/install.md)
- [Architecture](./docs/architecture.md)
- [Threat model](./docs/threat-model.md)
- [Validation](./docs/validation.md)
- [Release checklist](./docs/release.md)
- [Security policy](./SECURITY.md)
- [Contributing](./CONTRIBUTING.md)

## Release

```bash
make release-snapshot
make checksum
make sbom
```

`make release-snapshot` uses GoReleaser when it is installed. Without GoReleaser it still builds `dist/hostshift`, writes `dist/checksums.txt`, and generates `dist/hostshift.sbom.spdx.json` from the Go module graph. Tagged GitHub releases use [.github/workflows/release.yml](./.github/workflows/release.yml).

Before publishing a public tag, complete [docs/release.md](./docs/release.md): hosted CI candidate, self-hosted or local VM apply, clean working tree, and signed checksum verification.

## Status

HostShift is in the validation milestone. The Go `hostshift` CLI is the primary implementation. The legacy Node CLI remains as the v0.2 behavior reference until parity is complete. Cloud and DNS automation are intentionally outside the core.

## License

Apache-2.0. See [LICENSE](./LICENSE).
