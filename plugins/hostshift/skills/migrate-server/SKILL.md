---
name: migrate-server
description: Safely discover, plan, prepare, stream, verify, and audit Ubuntu or Debian web server migrations with HostShift while enforcing a strictly read-only source policy. Use when Codex is asked to migrate, clone, reproduce, inventory, compare, or check drift for Linux servers running web applications, Docker Compose, standalone Docker containers, systemd services, Nginx, Apache, Caddy, PHP-FPM, Supervisor, Fail2ban, Memcached, RabbitMQ, Certbot/Let's Encrypt, Logrotate, MySQL/MariaDB, PostgreSQL, Redis, firewall rules, SSH settings, or TLS certificates.
---

# Migrate Server

Use the `hostshift` CLI when it is installed on `PATH`. When working inside the HostShift repository, use `go run ./cmd/hostshift` from the repository root or `./dist/hostshift` after `make build`.

The skill is an operator layer. The migration engine is the deterministic HostShift CLI.

## Safety Rules

1. Never run arbitrary commands on the source.
2. Never use `sudo`, install packages, write files, create dumps on disk, manage services, signal processes, alter firewall rules, add SSH keys, create snapshots, or place apps into maintenance mode on the source.
3. Never claim point-in-time filesystem consistency when the source application remains writable.
4. Stop before `--apply` unless the profile is reviewed, `approved` is `true`, and the plan has no blockers.
5. Keep DNS and cloud-provider changes manual unless a future explicit plugin handles them.
6. Run mutations only through target-side actions.
7. Do not print `.env`, Compose files, Docker inspect output, database dumps, or credentials verbatim.

Read [references/safety-model.md](references/safety-model.md) before changing source command definitions or transfer behavior.

## Workflow

Prefer dry-run commands first:

```bash
hostshift doctor --source old-server --target new-server --json
hostshift capabilities --json
hostshift discover --source old-server --name example --profile example.profile.yaml --json
hostshift plan --profile example.profile.yaml --target new-server --json
hostshift explain --profile example.profile.yaml --target new-server --json
hostshift review --profile example.profile.yaml --target new-server --json
hostshift prepare --profile example.profile.yaml --target new-server --json
hostshift sync --profile example.profile.yaml --target new-server --json
hostshift verify --profile example.profile.yaml --target new-server --json
hostshift status --state-dir .hostshift --run-id sync-001 --json
hostshift resume --profile example.profile.yaml --state-dir .hostshift --run-id sync-001 --json
```

Use `--apply` only after displaying the exact target mutations and source read commands to the user. Run application checks before any manual DNS change. Run drift/status checks after migration when available.

If the `hostshift` binary is missing, stop and install it from the HostShift release archive or build it from source. Do not silently fall back to ad hoc shell commands.

For interrupted runs, preview `resume` without `--apply` first. Resume apply must use the same reviewed profile and plan fingerprint. Never retry a failed or uncertain action unless the operator has inspected the target and explicitly supplies the exact `--retry-failed <action-id>` value.

## Platform Guidance

- Prefer Ubuntu LTS or Debian stable targets.
- Treat EOL targets as blockers by default.
- Allow EOL sources only for read-only export when the migration can still be verified.
- Do not require source and target versions to match. Require compatibility checks instead.

## Workload Guidance

- Detect Docker Compose projects, standalone containers, bind mounts, and named volumes.
- Model file transfers as `tar --create --file=-` on source into `tar --extract --file=-` on target. Do not create source-side archives.
- Treat named volumes as blockers until an explicit `snapshot`, `disposable`, `database-backed`, or `external` strategy is reviewed. Snapshot mode may only read an existing source tar; HostShift must never create it.
- Use source `docker exec` only through typed read-only dump operations. Never open arbitrary shell access.
- Treat stream actions as source stdout to target stdin. The source side must stay read-only and the target side may mutate only the target.
- Prefer MySQL single-transaction streaming and PostgreSQL custom-format streaming.
- Model database passwords as environment variable names such as `sourcePasswordEnv` and `targetPasswordEnv`; never place literal passwords in profiles or commands.
- For Redis, require an existing RDB snapshot or read-only replica stream; never create source-side snapshots.
- Preserve SSH, firewall, Fail2ban, Memcached, RabbitMQ, Certbot/Let's Encrypt, Logrotate, MySQL bind settings, Nginx/Apache/Caddy/PHP-FPM/Supervisor config, and application-level checks.
- Verify HTTP endpoints with reviewed URLs and Host headers. Verify Laravel database connectivity with the fixed target-side PDO probe.
