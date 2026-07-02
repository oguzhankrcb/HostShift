#!/bin/sh
set -eu

mkdir -p /run/mysqld
chown mysql:mysql /run/mysqld

if [ ! -d /var/lib/mysql/mysql ]; then
  mariadb-install-db --user=mysql --datadir=/var/lib/mysql >/tmp/hostshift-source-mariadb-init.log 2>&1
fi

mariadbd --user=mysql --datadir=/var/lib/mysql --socket=/run/mysqld/mysqld.sock --pid-file=/run/mysqld/mysqld.pid --bind-address=127.0.0.1 >/tmp/hostshift-source-mariadb.log 2>&1 &
MYSQL_PID=$!

ready=0
for _ in $(seq 1 60); do
  if mysqladmin ping --silent >/dev/null 2>&1; then
    ready=1
    break
  fi
  sleep 1
done
if [ "$ready" -ne 1 ]; then
  echo "mariadb did not become ready" >&2
  cat /tmp/hostshift-source-mariadb.log >&2 || true
  exit 1
fi

if ! mysql --batch --skip-column-names --execute="SHOW DATABASES LIKE 'fixturedb'" | grep -qx fixturedb; then
  mysql < /srv/app/fixtures/mysql/fixturedb.sql
fi

PG_CLUSTER_INFO="$(pg_lsclusters --no-header | awk 'NR==1 {print $1 " " $2}')"
if [ -z "$PG_CLUSTER_INFO" ]; then
  echo "postgres cluster metadata is missing" >&2
  exit 1
fi
set -- $PG_CLUSTER_INFO
PG_VERSION="$1"
PG_CLUSTER="$2"
pg_ctlcluster --skip-systemctl-redirect "$PG_VERSION" "$PG_CLUSTER" start >/tmp/hostshift-source-postgres.log 2>&1

pg_ready=0
for _ in $(seq 1 60); do
  if runuser -u postgres -- psql --tuples-only --no-align --command "SELECT 1" postgres >/dev/null 2>&1; then
    pg_ready=1
    break
  fi
  sleep 1
done
if [ "$pg_ready" -ne 1 ]; then
  echo "postgres did not become ready" >&2
  cat /tmp/hostshift-source-postgres.log >&2 || true
  exit 1
fi

if ! runuser -u postgres -- psql --tuples-only --no-align --command "SELECT 1 FROM pg_roles WHERE rolname='root'" postgres | grep -qx 1; then
  runuser -u postgres -- createuser -s root
fi

if ! runuser -u postgres -- psql --tuples-only --no-align --command "SELECT 1 FROM pg_database WHERE datname='fixturepg'" postgres | grep -qx 1; then
  runuser -u postgres -- createdb -O root fixturepg
fi

if ! psql --username root --dbname fixturepg --tuples-only --no-align --command "SELECT to_regclass('public.metrics') IS NOT NULL" | grep -qx t; then
  psql --username root --dbname fixturepg --file /srv/app/fixtures/postgresql/fixturedb.sql >/tmp/hostshift-source-postgres-import.log 2>&1
fi

/usr/sbin/sshd -D -e &
SSHD_PID=$!

cleanup() {
  pg_ctlcluster --skip-systemctl-redirect "$PG_VERSION" "$PG_CLUSTER" stop >/tmp/hostshift-source-postgres-stop.log 2>&1 || true
  kill "$SSHD_PID" "$MYSQL_PID" 2>/dev/null || true
  wait "$SSHD_PID" "$MYSQL_PID" 2>/dev/null || true
}
trap cleanup INT TERM

wait "$SSHD_PID"
