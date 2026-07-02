---
title: Source Safety
description: The read-only-source contract.
---

HostShift treats the source host as read-only infrastructure. This is not a convention; it is encoded into the planner, source command allowlist, executor behavior, and test matrix.

## Allowed On Source

- fact collection such as `/etc/os-release`
- read-only Docker metadata queries
- `tar --create --file=-` streams
- `docker image save` streams
- `mysqldump` streams
- `pg_dump` streams

## Forbidden On Source

- `sudo`
- package installation
- writing files
- service start, stop, restart, reload, or disable operations
- firewall changes
- snapshot creation
- maintenance mode
- key creation or modification
- machine identity transfer

## How Violations Are Caught

The safety layer rejects mutation-like source commands. The integration and VM matrices also capture source snapshots and verify that the source fixture state does not change after migration.

When HostShift cannot safely read a workload online, it must produce a blocker instead of pretending the workload was migrated.
