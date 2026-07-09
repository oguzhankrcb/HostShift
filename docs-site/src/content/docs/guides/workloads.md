---
title: Workloads
description: Supported workload and verification adapters.
---

HostShift uses workload adapters to plan transfer, target preparation, verification, and rollback metadata.

## Supported Workloads

- `docker-compose`
- `docker-standalone`
- `file-set`
- `cron`
- `php-fpm`
- `supervisor`
- `fail2ban`
- `memcached`
- `rabbitmq`
- `certbot`
- `logrotate`
- `caddy`
- `apache-vhost`
- `systemd-service`
- `mysql`
- `mariadb`
- `postgresql`
- `redis`

For every field, generated command, target package capability, and validation rule, see [Workload Reference](/reference/workloads/).

## Transfer Strategy

Sync plans may include `streams`, which pipe a validated source read command into a validated target write command.

Examples:

- `tar --create` to `tar --extract`
- `docker image save` to `docker image load`
- `mysqldump` to `mysql`
- `pg_dump` to `pg_restore`
- existing Redis RDB or `redis-cli --rdb` stream to target RDB file

## Checks

Verify checks include:

- HTTP status checks with optional Host header
- Laravel database connectivity through a target-side container command
- target-side `fileExists` and `fileContains`
- MySQL and PostgreSQL scalar checks
- systemd `serviceActive`
- UFW and nftables rule checks
- Nginx config validation and reload

For every check type and its generated target command, see [Check Reference](/reference/checks/).

## Target Package Planning

HostShift derives package requirements from workloads and checks, maps them through the Ubuntu/Debian platform adapter, and blocks unknown target platforms instead of guessing package names.
