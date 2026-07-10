---
title: Workload Reference
description: Supported migration workload types and their generated actions or streams.
---

Workloads describe what HostShift should prepare or move. Every source-side movement is represented as a stream from a validated read command into a target write command.

## docker-compose

Validates a Compose project on the target.

```yaml
- type: docker-compose
  name: customer-app
  data:
    workingDir: /srv/customer-app
    configFile: /srv/customer-app/docker-compose.yml
```

Fields:

- `workingDir`: optional absolute directory. If set, HostShift runs the Compose command from this directory.
- `configFile`: optional absolute Compose file path.

Generated prepare action:

```text
docker compose -f <configFile> config
```

When `workingDir` is set, the command is shell-wrapped as `cd <workingDir> && docker compose ...`.

Target capabilities:

- `docker-runtime`
- `docker-compose`

## docker-standalone

Streams a standalone image from source to target.

```yaml
- type: docker-standalone
  name: fixture-standalone
  data:
    image: fixture/standalone:latest
```

Fields:

- `image`: optional image reference. Defaults to the workload name.

Generated sync stream:

```text
source: docker image save <image>
target: docker image load
```

Target capability:

- `docker-runtime`

## docker-volume

Models every discovered Docker named volume as an explicit operator decision. Discovery emits the workload without a strategy, which blocks apply until it is reviewed.

Existing snapshot strategy:

```yaml
- type: docker-volume
  name: uploads
  data:
    volumeName: uploads
    strategy: snapshot
    snapshotPath: /srv/hostshift-snapshots/uploads.tar
    targetPath: /srv/hostshift/volumes/uploads
```

Non-copy strategies:

```yaml
- type: docker-volume
  name: cache
  data:
    strategy: disposable

- type: docker-volume
  name: database-data
  data:
    strategy: database-backed

- type: docker-volume
  name: shared-media
  data:
    strategy: external
```

Fields:

- `volumeName`: source volume name recorded by discovery.
- `strategy`: required before apply. Accepted values are `snapshot`, `disposable`, `database-backed`, and `external`.
- `snapshotPath`: required for `snapshot`; an existing source tar file produced outside HostShift.
- `targetPath`: optional extraction directory. Defaults to `/srv/hostshift/volumes/<workload-name>`.

Generated snapshot stream:

```text
source: cat <snapshotPath>
target: test ! -e <targetPath> && install -d <targetPath> && tar --extract --file=- --no-same-owner -C <targetPath>
```

HostShift never runs a source-side volume container, archive command, or snapshot operation. Snapshot restore refuses to merge into an existing target path, which keeps its manual rollback metadata scoped to a directory HostShift created. The target path is a staging or bind-mount data directory; the operator must wire it into the target Compose or container definition. `disposable`, `database-backed`, and `external` produce no data stream and remain visible in review output so skipped data cannot be mistaken for migrated data.

Target capability for `snapshot`:

- `tar`

## file-set

Streams files or directories with tar.

```yaml
- type: file-set
  name: web-files
  data:
    paths:
      - /srv/customer-app
      - /etc/nginx/sites-available/customer.conf
    targetPath: /
```

Fields:

- `paths`: required list of absolute source paths.
- `targetPath`: optional absolute target extraction path. Defaults to `/`.

Generated sync stream:

```text
source: tar --create --file=- --one-file-system --warning=no-file-changed -C / <relative paths>
target: tar --extract --file=- -C <targetPath>
```

Target capabilities:

- `tar`
- `nginx` when any path includes `/etc/nginx`
- `apache` when any path includes `/etc/apache2`
- `caddy` when any path includes `/etc/caddy`
- `cron` when any path includes `/etc/cron.d`, `/etc/cron.daily`, `/etc/cron.hourly`, `/etc/cron.monthly`, or `/etc/cron.weekly`
- `php-fpm` when PHP-FPM configuration under `/etc/php` is paired with a `php-fpm` workload
- `supervisor` when any path includes `/etc/supervisor`
- `fail2ban` when any path includes `/etc/fail2ban`
- `memcached` when any path includes `/etc/memcached.conf` or `/etc/memcached`
- `rabbitmq-server` when any path includes `/etc/rabbitmq`
- `certbot` when any path includes `/etc/letsencrypt`
- `logrotate` when any path includes `/etc/logrotate.conf` or `/etc/logrotate.d`

HostShift rejects broad or machine-identity paths through the transfer path safety rules.

## cron

Reloads cron after cron files have been synced to the target.

```yaml
- type: cron
  name: cron
  data:
    service: cron.service
```

Fields:

- `service`: optional cron service name. Defaults to distro fallbacks for `cron` and `crond`.

Generated cutover action without `service`:

```text
systemctl reload cron || systemctl restart cron || systemctl reload crond || systemctl restart crond
```

Discovery suggests this workload when cron files are discovered under `/etc/cron.d`, `/etc/cron.daily`, `/etc/cron.hourly`, `/etc/cron.monthly`, or `/etc/cron.weekly`. The source remains read-only; only the target service is reloaded or restarted.

Target capability:

- `cron`

## php-fpm

Reloads or restarts PHP-FPM after PHP configuration files have been synced to the target.

```yaml
- type: php-fpm
  name: php8.3-fpm
  data:
    service: php8.3-fpm.service
```

Fields:

- `service`: optional PHP-FPM service name. Defaults to the workload name.

Generated cutover action:

```text
systemctl reload <service> || systemctl restart <service>
```

Discovery suggests this workload when PHP-FPM services or packages are discovered. If `/etc/php` files are readable, discovery also suggests a `file-set` for those PHP configuration files. The source remains read-only; only the target service is reloaded or restarted.

Target capability:

- `php-fpm`

## supervisor

Updates Supervisor after Supervisor configuration files have been synced to the target.

```yaml
- type: supervisor
  name: supervisor
  data:
    service: supervisor.service
```

Fields:

- `service`: optional Supervisor service name. Defaults to `supervisor.service`.

Generated cutover action:

```text
systemctl enable --now <service> && supervisorctl reread && supervisorctl update
```

Discovery suggests this workload when `/etc/supervisor` files, `supervisor.service`, or the `supervisor` package are discovered. If `/etc/supervisor` files are readable, discovery also suggests a `file-set` for those Supervisor configuration files. The source remains read-only; only the target process supervisor is enabled and updated.

Target capability:

- `supervisor`

## fail2ban

Reloads Fail2ban after Fail2ban configuration files have been synced to the target.

```yaml
- type: fail2ban
  name: fail2ban
  data:
    service: fail2ban.service
```

Fields:

- `service`: optional Fail2ban service name. Defaults to `fail2ban.service`.

Generated cutover action:

```text
systemctl enable --now <service> && (fail2ban-client reload || systemctl restart <service>)
```

Discovery suggests this workload when `/etc/fail2ban` files, `fail2ban.service`, or the `fail2ban` package are discovered. If `/etc/fail2ban` files are readable, discovery also suggests a `file-set` for those Fail2ban configuration files. The source remains read-only; only the target intrusion-prevention service is enabled and reloaded or restarted.

Target capability:

- `fail2ban`

## logrotate

Validates Logrotate configuration after Logrotate files have been synced to the target.

```yaml
- type: logrotate
  name: logrotate
  data:
    config: /etc/logrotate.conf
```

Fields:

- `config`: optional Logrotate config path. Defaults to `/etc/logrotate.conf`.

Generated verify action:

```text
logrotate --debug <config>
```

Discovery suggests this workload when `/etc/logrotate.conf`, `/etc/logrotate.d` files, or the `logrotate` package are discovered. If Logrotate config files are readable, discovery also suggests a `file-set` for those files. The source remains read-only; the target action validates config parsing without rotating logs.

Target capability:

- `logrotate`

## caddy

Validates and reloads Caddy after Caddy configuration files have been synced to the target.

```yaml
- type: caddy
  name: caddy
  data:
    service: caddy.service
    config: /etc/caddy/Caddyfile
```

Fields:

- `service`: optional Caddy service name. Defaults to `caddy.service`.
- `config`: optional Caddy config path. Defaults to `/etc/caddy/Caddyfile`.

Generated verify action:

```text
caddy validate --config <config> && (systemctl reload <service> || systemctl restart <service>)
```

Discovery suggests this workload when `/etc/caddy` files, `caddy.service`, or the `caddy` package are discovered. If `/etc/caddy` files are readable, discovery also suggests a `file-set` for those Caddy configuration files. The source remains read-only; only the target web server config is validated and reloaded or restarted.

Target capability:

- `caddy`

## memcached

Restarts Memcached on the target after configuration files have been synced.

```yaml
- type: memcached
  name: memcached
  data:
    service: memcached.service
    config: /etc/memcached.conf
```

Fields:

- `service`: optional Memcached service name. Defaults to `memcached.service`.
- `config`: optional Memcached config path. Defaults to `/etc/memcached.conf`.

Generated cutover action:

```text
test -f <config> && systemctl enable --now <service> && systemctl restart <service>
```

Discovery suggests this workload when `/etc/memcached.conf`, `/etc/memcached` files, `memcached.service`, or the `memcached` package are discovered. If Memcached config files are readable, discovery also suggests a `file-set` for those files. The source remains read-only; HostShift does not attempt to preserve volatile in-memory cache contents.

Target capability:

- `memcached`

## rabbitmq

Restarts RabbitMQ on the target after configuration files have been synced.

```yaml
- type: rabbitmq
  name: rabbitmq
  data:
    service: rabbitmq-server.service
    configDir: /etc/rabbitmq
```

Fields:

- `service`: optional RabbitMQ service name. Defaults to `rabbitmq-server.service`.
- `configDir`: optional RabbitMQ config directory. Defaults to `/etc/rabbitmq`.

Generated cutover action:

```text
test -d <configDir> && systemctl enable --now <service> && systemctl restart <service> && rabbitmq-diagnostics check_running && rabbitmq-diagnostics check_local_alarms
```

Discovery suggests this workload when `/etc/rabbitmq` files, `rabbitmq-server.service`, or the `rabbitmq-server` package are discovered. If RabbitMQ config files are readable, discovery also suggests a `file-set` for those files. The source remains read-only. HostShift does not migrate live queues, messages, or RabbitMQ node state with this workload.

Target capability:

- `rabbitmq-server`

## certbot

Validates migrated Let's Encrypt state on the target and enables the Certbot renewal timer when the target package provides one.

```yaml
- type: certbot
  name: certbot
  data:
    configDir: /etc/letsencrypt
```

Fields:

- `configDir`: optional Certbot/Let's Encrypt config directory. Defaults to `/etc/letsencrypt`.

Generated cutover action:

```text
test -d <configDir> && certbot certificates >/dev/null && (systemctl list-unit-files certbot.timer >/dev/null 2>&1 && systemctl enable --now certbot.timer || true)
```

Discovery suggests this workload when `/etc/letsencrypt` files or the `certbot` package are discovered. If Let's Encrypt files are readable, discovery also suggests a `file-set` for `/etc/letsencrypt`. The source remains read-only. HostShift preserves existing certificate state only; DNS routing, ACME challenge behavior, and future renewal behavior require operator review.

Target capability:

- `certbot`

## apache-vhost

Enables Apache modules and sites after Apache config files have been synced, validates the target config, and reloads Apache.

```yaml
- type: apache-vhost
  name: customer-apache
  data:
    modules:
      - rewrite
      - ssl
    sites:
      - customer.conf
```

Generated verify action:

```text
a2enmod <module> && a2ensite <site> && apache2ctl configtest && (systemctl reload apache2 || systemctl restart apache2)
```

When Apache config is migrated, HostShift also plans a target-only prepare action to disable the packaged `000-default.conf` site.

Target capability:

- `apache`

## systemd-service

Enables and starts an application service during cutover.

```yaml
- type: systemd-service
  name: customer-worker
  data:
    service: customer-worker.service
    unitPath: /etc/systemd/system/customer-worker.service
```

Fields:

- `service`: optional service name. Defaults to the workload name.
- `unitPath`: optional expected unit file path. If set, it must be under `/etc/systemd/system/` and end with `.service`.

Generated cutover action:

```text
systemctl daemon-reload && systemctl enable --now <service>
```

Rollback metadata:

```text
systemctl disable --now <service> || true
```

Discovery can suggest `systemd-service` for custom unit files under `/etc/systemd/system` when the matching service is enabled. Operators must still review these candidates before setting `approved: true`.

## mysql

Streams a MySQL database dump.

```yaml
- type: mysql
  name: customer_db
  data:
    sourcePasswordEnv: SRC_MYSQL_PWD
    targetPasswordEnv: DST_MYSQL_PWD
```

Fields:

- workload `name`: database name.
- `sourcePasswordEnv`: optional source-side env var name.
- `targetPasswordEnv`: optional target-side env var name.

Generated sync stream:

```text
source: mysqldump --single-transaction --quick --skip-lock-tables --databases <name>
target: mysql
```

The source command includes `--no-tablespaces` when supported. The target command filters known MySQL 8 collation and encryption syntax that breaks older targets.

Target capability:

- `mysql-client`

## mariadb

Uses the same stream strategy as `mysql`, but maps target capability to MariaDB client packages where applicable.

```yaml
- type: mariadb
  name: customer_db
```

Target capability:

- `mariadb-client`

## postgresql

Streams a PostgreSQL custom-format dump.

```yaml
- type: postgresql
  name: customer_pg
  data:
    sourcePasswordEnv: SRC_PG_PWD
    targetPasswordEnv: DST_PG_PWD
```

Generated sync stream without target password env:

```text
source: pg_dump --format=custom --dbname <name>
target: createdb if missing, then runuser -u postgres -- pg_restore --clean --if-exists --no-owner --no-acl
```

When `targetPasswordEnv` is set, HostShift uses password-based `pg_restore` directly.

Target capabilities:

- `postgresql-server`
- `postgresql-client`

## redis

Streams Redis data without creating a source-side snapshot.

Existing snapshot strategy:

```yaml
- type: redis
  name: cache
  data:
    snapshotPath: /var/lib/redis/dump.rdb
    targetPath: /var/lib/redis/dump.rdb
```

Replica stream strategy:

```yaml
- type: redis
  name: cache
  data:
    replicaHost: 127.0.0.1
    replicaPort: 6380
    targetPath: /var/lib/redis/dump.rdb
```

Fields:

- `snapshotPath`: existing source RDB file. HostShift reads it with `cat`; it does not run `SAVE` or `BGSAVE`.
- `replicaHost`: Redis replica endpoint used with `redis-cli --rdb -`.
- `replicaPort`: optional replica port. Defaults to `6379`.
- `targetPath`: optional target RDB path. Defaults to `/var/lib/redis/dump.rdb`.

Exactly one of `snapshotPath` or `replicaHost` must be set. If neither is set, or both are set, the plan reports a blocker.

Generated sync stream for an existing snapshot:

```text
source: cat <snapshotPath>
target: install -d <target directory> && cat > <targetPath>
```

Generated sync stream for a replica:

```text
source: redis-cli -h <replicaHost> -p <replicaPort> --rdb -
target: install -d <target directory> && cat > <targetPath>
```

Generated cutover action:

```text
systemctl restart redis-server || systemctl restart redis
```

Target capabilities:

- `redis-server`
- `redis-tools`

## Unknown Workloads

Unknown workload types are represented as local read-only inspect actions during planning:

```text
hostshift inspect-workload <type>
```

They do not silently apply target mutations.
