---
title: Workloads
description: Supported workload and verification adapters.
---

HostShift uses workload adapters to plan transfer, target preparation, verification, and rollback metadata.

## Supported Workloads

- `docker-compose`
- `docker-standalone`
- `file-set`
- `mysql`
- `postgresql`

## Transfer Strategy

Sync plans may include `streams`, which pipe a validated source read command into a validated target write command.

Examples:

- `tar --create` to `tar --extract`
- `docker image save` to `docker image load`
- `mysqldump` to `mysql`
- `pg_dump` to `pg_restore`

## Checks

Verify checks include:

- HTTP status checks with optional Host header
- Laravel database connectivity through a target-side container command
- target-side `fileExists` and `fileContains`
- MySQL and PostgreSQL scalar checks
- systemd `serviceActive`
- UFW and nftables rule checks
- Nginx config validation and reload

## Target Package Planning

HostShift derives package requirements from workloads and checks, maps them through the Ubuntu/Debian platform adapter, and blocks unknown target platforms instead of guessing package names.
