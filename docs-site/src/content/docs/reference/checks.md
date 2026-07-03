---
title: Check Reference
description: Supported target verification checks.
---

Checks run during `verify`. They are target-side assertions and do not modify the source.

## http

Checks an HTTP or HTTPS endpoint.

```yaml
- type: http
  name: public-health
  data:
    url: http://127.0.0.1/health
    hostHeader: example.com
    timeoutSeconds: 10
```

Fields:

- `url`: required `http` or `https` URL.
- `hostHeader`: optional Host header.
- `timeoutSeconds`: optional 1-300 seconds. Defaults to 10.

Generated command:

```text
curl --fail --silent --show-error --max-time <timeout> [--header "Host: <hostHeader>"] <url>
```

Target capability:

- `curl`

## laravelDatabase

Checks that a Laravel app container can open its configured database connection.

```yaml
- type: laravelDatabase
  name: app-db
  data:
    container: customer-app
```

Generated command:

```text
docker exec <container> php artisan tinker --execute=DB::connection()->getPdo(); echo 'hostshift-db-ok';
```

Target capability:

- `docker-runtime`

## fileExists

Checks that a target file exists.

```yaml
- type: fileExists
  name: nginx-site
  data:
    path: /etc/nginx/sites-available/customer.conf
```

Generated command:

```text
test -f <path>
```

## fileContains

Checks that a target file contains a literal string.

```yaml
- type: fileContains
  name: ssh-keepalive
  data:
    path: /etc/ssh/sshd_config.d/99-hostshift.conf
    contains: ClientAliveInterval 120
```

Generated command:

```text
grep -Fq -- <contains> <path>
```

The `contains` value must be a non-empty single-line literal.

## mysqlScalar

Runs a single read-only MySQL `SELECT` and compares the scalar result.

```yaml
- type: mysqlScalar
  name: page-count
  data:
    database: customer_db
    query: SELECT COUNT(*) FROM pages
    expected: "42"
```

Rules:

- `query` must be one single-line `SELECT`.
- semicolons and SQL mutation tokens are rejected.
- `expected` must be a non-empty single-line string.

Generated command:

```text
mysql --batch --skip-column-names --database=<database> --execute=<query>
```

Target capability:

- `mysql-client`

## postgresScalar

Runs a single read-only PostgreSQL `SELECT` and compares the scalar result.

```yaml
- type: postgresScalar
  name: metric-count
  data:
    database: customer_pg
    query: SELECT COUNT(*) FROM metrics
    expected: "2"
```

Generated command:

```text
runuser -u postgres -- psql --tuples-only --no-align --dbname=<database> --command=<query>
```

Target capabilities:

- `postgresql-server`
- `postgresql-client`

## serviceActive

Checks a systemd service is active.

```yaml
- type: serviceActive
  name: nginx-service
  data:
    service: nginx
```

Generated command:

```text
systemctl is-active --quiet <service>
```

## ufwRule

Checks that a target UFW rule exists.

```yaml
- type: ufwRule
  name: mysql-rule
  data:
    from: 172.17.0.0/16
    port: 3306
    proto: tcp
```

Generated command:

```text
ufw show added | grep -Fq -- "ufw allow from <from> to any port <port> proto <proto>"
```

## nftRule

Checks that a target nftables chain contains a literal rule fragment.

```yaml
- type: nftRule
  name: mysql-nft
  data:
    family: inet
    table: hostshift
    chain: input
    contains: tcp dport 3306 accept
```

Rules:

- `family` must be `inet`, `ip`, or `ip6`.
- `table` and `chain` must be safe identifiers.
- `contains` must be a non-empty single-line literal.

## nginxConfig

Validates Nginx configuration and reloads or restarts Nginx on the target.

```yaml
- type: nginxConfig
  name: reload-nginx
```

Generated command:

```text
nginx -t && (systemctl reload nginx || systemctl restart nginx)
```

This check has `service` impact because it can reload the target service.

