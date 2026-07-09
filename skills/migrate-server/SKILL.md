---
name: migrate-server
description: Safely discover, plan, prepare, stream, verify, and audit Ubuntu or Debian web server migrations with HostShift while enforcing a strictly read-only source policy. Use when Codex is asked to migrate, clone, reproduce, inventory, compare, or check drift for Linux servers running web applications, Docker Compose, standalone Docker containers, systemd services, Nginx, Apache, Caddy, PHP-FPM, Supervisor, Fail2ban, Memcached, RabbitMQ, Certbot/Let's Encrypt, Logrotate, MySQL/MariaDB, PostgreSQL, Redis, firewall rules, SSH settings, or TLS certificates.
---

# Migrate Server

Use the bundled `hostshift` CLI. HostShift migration behavior lives in the Go binary; do not fall back to removed Node entrypoints or ad hoc shell migrations.

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

Run from the plugin root:

```bash
go run ./cmd/hostshift doctor --source old-server --target new-server --json
go run ./cmd/hostshift capabilities --json
go run ./cmd/hostshift plan --profile example.profile.yaml --target new-server --json
go run ./cmd/hostshift explain --profile example.profile.yaml --target new-server --json
go run ./cmd/hostshift review --profile example.profile.yaml --target new-server --json
go run ./cmd/hostshift profile migrate --input old-v1.profile.yaml --output new-v2.profile.json
```

Use `--apply` only after displaying the exact target mutations and source read commands to the user. Run application checks before any manual DNS change. Use `verify`, `status`, and audit logs after migration.

## Platform Guidance

- Prefer Ubuntu LTS or Debian stable targets.
- Treat EOL targets as blockers by default.
- Allow EOL sources only for read-only export when the migration can still be verified.
- Do not require source and target versions to match. Require compatibility checks instead.

## Workload Guidance

- Detect Docker Compose projects, standalone containers, bind mounts, and named volumes.
- Model file transfers as `tar --create --file=-` on source into `tar --extract --file=-` on target. Do not create source-side archives.
- Treat named volumes as blockers until an explicit strategy marks them disposable, database-backed, or safely exportable.
- Use source `docker exec` only through typed read-only dump operations. Never open arbitrary shell access.
- Treat stream actions as source stdout to target stdin. The source side must stay read-only and the target side may mutate only the target.
- Prefer MySQL single-transaction streaming and PostgreSQL custom-format streaming.
- Model database passwords as environment variable names such as `sourcePasswordEnv` and `targetPasswordEnv`; never place literal passwords in profiles or commands.
- For Redis, require an existing RDB snapshot or read-only replica stream; never create source-side snapshots.
- Preserve SSH, firewall, Fail2ban, Memcached, RabbitMQ, Certbot/Let's Encrypt, Logrotate, MySQL bind settings, Nginx/Apache/Caddy/PHP-FPM/Supervisor config, and application-level checks.
- Verify HTTP endpoints with reviewed URLs and Host headers. Verify Laravel database connectivity with the fixed target-side PDO probe.
