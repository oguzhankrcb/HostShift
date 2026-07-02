# Threat Model

## Protected Asset

The primary protected asset is the live source server.

## Non-Negotiable Invariant

HostShift must not require source-side writes, package installs, service changes, firewall changes, key changes, snapshots, or application maintenance mode.

## Known Limits

- Reading large data can consume CPU, disk I/O, and network.
- Live filesystem streams are not point-in-time consistent.
- Database exports are only as consistent as their engine and selected dump mode.
- Redis is blocked unless an existing snapshot or replica can be read without changing the source.
- Docker named volumes are blockers unless a typed strategy is selected.
