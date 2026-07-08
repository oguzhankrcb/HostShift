---
title: Platform Support
description: Supported Ubuntu and Debian targets, package capabilities, and blockers.
---

HostShift uses platform adapters to avoid guessing target package names or lifecycle behavior.

## Supported Releases

The current platform catalog includes:

| Platform | Release | Status kind | Standard EOL |
| --- | --- | --- | --- |
| Ubuntu | 22.04 | LTS | 2027-05-31 |
| Ubuntu | 24.04 | LTS | 2029-05-31 |
| Ubuntu | 25.10 | interim | 2026-07-31 |
| Ubuntu | 26.04 | LTS | 2031-05-31 |
| Debian | 12 | LTS | 2028-06-30 |
| Debian | 13 | standard | 2028-06-30 |

EOL targets are blocked. EOL sources may still be read when the operation remains source-safe.

## Cross-Distribution Behavior

Cross-distribution migrations are allowed but produce warnings:

```text
Cross-distribution migration ubuntu:22.04 -> debian:12 requires workload compatibility checks
```

The warning is intentional. HostShift can stream files and data, but application runtime compatibility still belongs in checks.

## Package Capabilities

HostShift plans target package installation from abstract capabilities:

| Capability | Ubuntu package | Debian package |
| --- | --- | --- |
| `rsync` | `rsync` | `rsync` |
| `tar` | `tar` | `tar` |
| `curl` | `curl` | `curl` |
| `docker-runtime` | `docker.io` | `docker.io` |
| `docker-compose` | `docker-compose-plugin` | `docker-compose-plugin` |
| `nginx` | `nginx` | `nginx` |
| `apache` | `apache2` | `apache2` |
| `caddy` | `caddy` | `caddy` |
| `cron` | `cron` | `cron` |
| `php-fpm` | `php-fpm` | `php-fpm` |
| `supervisor` | `supervisor` | `supervisor` |
| `fail2ban` | `fail2ban` | `fail2ban` |
| `logrotate` | `logrotate` | `logrotate` |
| `openssh-server` | `openssh-server` | `openssh-server` |
| `mysql-server` | `mysql-server` | `default-mysql-server` |
| `mysql-client` | `mysql-client` | `default-mysql-client` |
| `mariadb-client` | `mariadb-client` | `mariadb-client` |
| `postgresql-server` | `postgresql` | `postgresql` |
| `postgresql-client` | `postgresql-client` | `postgresql-client` |
| `redis-server` | `redis-server` | `redis-server` |
| `redis-tools` | `redis-tools` | `redis-tools` |
| `nftables` | `nftables` | `nftables` |
| `ufw` | `ufw` | `ufw` |

Unknown platforms or missing capability mappings become blockers instead of best-effort installs.

## Service And Firewall Model

Ubuntu and Debian adapters currently assume:

- package manager: `apt`
- service manager: `systemd`
- firewall backends: UFW and nftables

Docker tests cover fast SSH-driven migration behavior. VM tests cover package install, systemd, firewall, and reboot persistence behavior.
