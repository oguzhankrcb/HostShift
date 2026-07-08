---
title: Source Discovery
description: Read-only source fact collection and generated profile behavior.
---

Source discovery is an allowlisted read-only operation. HostShift treats the source as an observation endpoint and never writes to it.

## Fact Execution

`hostshift discover` runs each fact through the source command guard. Required facts must succeed. Optional facts can fail and still be reported in the JSON output.

```bash
hostshift discover --source old-server --name migration --profile migration.profile.yaml --json
```

## Required Facts

Required facts currently include:

- `osRelease`: `cat /etc/os-release`
- `architecture`: `uname -m`
- `hostname`: `hostname`
- `disk`: `df -Pk`
- `memory`: `cat /proc/meminfo`
- `packages`: `dpkg-query -W -f=${binary:Package}\t${Version}\n`
- `mounts`: `findmnt --json --real`
- `users`: `getent passwd`
- `groups`: `getent group`

If any required fact fails, discovery fails instead of generating a misleading profile.

## Optional Facts

Optional facts provide workload hints:

- `enabledServices`
- `runningServices`
- `listeners`
- `ufwStatus`
- `nftRuleset`
- `sshdEffectiveConfig`
- `sshdConfig`
- `mysqlServerConfig`
- `mysqlDatabases`
- `postgresDatabases`
- `nginxConfigDump`
- `apacheConfigDump`
- `phpConfigPaths`
- `supervisorConfigPaths`
- `fail2banConfigPaths`
- `logrotateConfigPaths`
- `letsEncryptFiles`
- `cron`
- `customSystemdUnits`
- `dockerVersion`
- `dockerComposeProjects`
- `dockerContainers`
- `dockerNetworks`

Optional failures are visible in output and should be reviewed before migration.

## Generated Profile

Discovery writes a profile with:

- `schemaVersion: 2`
- `source.ssh` set to the discovered alias
- `sourcePolicy: strict-read-only`
- `approved: false`
- `platforms.source` populated from `/etc/os-release` when available
- safe workload candidates derived from reviewed facts
- empty checks

Generated workload candidates currently include:

- Docker Compose projects from `docker compose ls --format json`
- matching Compose working directories as `file-set` workloads
- standalone Docker containers from `docker ps --format`
- `/etc/nginx` when `nginx -T` succeeds
- `/etc/apache2` when `apache2ctl -S` succeeds
- an `apache-vhost` activation candidate when Apache config is discovered
- PHP configuration files under `/etc/php` when PHP-FPM config files are discovered
- `php-fpm` reload candidates when `php*-fpm.service` or `php*-fpm` packages are discovered
- Supervisor configuration files under `/etc/supervisor` when Supervisor config files are discovered
- `supervisor` update candidates when `/etc/supervisor` files, `supervisor.service`, or the `supervisor` package are discovered
- Fail2ban configuration files under `/etc/fail2ban` when Fail2ban config files are discovered
- `fail2ban` reload candidates when `/etc/fail2ban` files, `fail2ban.service`, or the `fail2ban` package are discovered
- Logrotate configuration files under `/etc/logrotate.conf` and `/etc/logrotate.d` when Logrotate config files are discovered
- `logrotate` validation candidates when `/etc/logrotate.conf`, `/etc/logrotate.d` files, or the `logrotate` package are discovered
- `/etc/letsencrypt` when certificate files are discovered
- cron files under `/etc/cron.d`, `/etc/cron.daily`, `/etc/cron.hourly`, `/etc/cron.monthly`, and `/etc/cron.weekly`
- a `cron` target reload candidate when cron files are discovered
- custom systemd unit files under `/etc/systemd/system`
- `systemd-service` cutover candidates for discovered custom units that are enabled
- `redis` candidates when `redis-server` is found through packages or systemd facts
- non-system MySQL/MariaDB databases
- non-system PostgreSQL databases

HostShift does not generate `systemd-service` workloads from distribution service lists alone because it cannot safely distinguish application units from system units without operator review.

Redis candidates intentionally do not include a default export strategy. Operators must add either `snapshotPath` for an existing RDB file or `replicaHost` for a read-only replica stream before approval.

Operators must still review the generated profile, fill in the target, add checks, remove unwanted candidates, add missing workload metadata such as password environment variable names, and set `approved: true` only after review.

## Source Immutability

Discovery does not use:

- `sudo`
- package installation
- service management
- file writes
- snapshot creation
- maintenance mode
- firewall changes

The same source command guard also protects source-side sync streams.
