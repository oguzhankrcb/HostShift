# Threat Model

## Protected Asset

The primary protected asset is the live source server.

## Non-Negotiable Invariant

HostShift must not require source-side writes, package installs, service changes, firewall changes, key changes, snapshots, or application maintenance mode.

Source execution is default-deny. Only exact fact commands and typed read-only export shapes are accepted; unknown executables, alternate subcommands, shell snippets, and additional arguments are rejected before SSH execution.

## Known Limits

- Reading large data can consume CPU, disk I/O, and network.
- Live filesystem streams are not point-in-time consistent.
- Database exports are only as consistent as their engine and selected dump mode.
- Redis is supported only through an existing RDB snapshot or a read-only replica stream; source-side `SAVE`, `BGSAVE`, and config changes remain forbidden.
- Docker named volumes are blockers unless `snapshot`, `disposable`, `database-backed`, or `external` is selected. Snapshot mode only reads an existing tar file; it never creates or refreshes a source-side snapshot. A snapshot can be stale or application-inconsistent, so the operator must verify how and when it was produced.
