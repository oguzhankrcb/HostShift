# Source Safety Model

## Invariant

The source server is an immutable observation endpoint. A successful migration must not depend on changing its filesystem, packages, accounts, keys, firewall, services, processes, databases, or runtime state.

## Enforcement

- Source access is modeled as fact names and typed read-only exports, not arbitrary commands.
- Source commands never include `sudo`.
- Source checks may inspect OS release, packages, firewall, sshd, database config, Docker metadata, and web server metadata.
- Source container access is allowed only through typed read-only export operations such as streaming a database dump to stdout.
- Dynamic transfer paths must be absolute, narrow, and outside machine-specific exclusions.
- Database exports write to stdout and never create source temporary files.
- Logs and profiles contain metadata and redacted commands, not secret values.

## Known Limits

- Reading large datasets consumes CPU, disk I/O, and network bandwidth.
- Live filesystem streaming is not a point-in-time snapshot.
- MySQL dumps can acquire metadata locks.
- PostgreSQL dumps are consistent for the selected database, not across unrelated databases.
- Redis cannot be safely migrated under this policy unless a usable snapshot or replica already exists.
- Docker named volumes that change during transfer may be inconsistent.

Report these effects before execution. Do not weaken the invariant to make a migration pass.
