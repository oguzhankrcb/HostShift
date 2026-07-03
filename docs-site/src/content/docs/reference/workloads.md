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
- `cron` when any path includes `/etc/cron.d`, `/etc/cron.daily`, `/etc/cron.hourly`, `/etc/cron.monthly`, or `/etc/cron.weekly`

HostShift rejects broad or machine-identity paths through the transfer path safety rules.

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

## Unknown Workloads

Unknown workload types are represented as local read-only inspect actions during planning:

```text
hostshift inspect-workload <type>
```

They do not silently apply target mutations.
