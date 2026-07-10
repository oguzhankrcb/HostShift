#!/usr/bin/env bash
set -euo pipefail

export DEBIAN_FRONTEND=noninteractive

HOSTSHIFT_LOGIN_USER="$(getent passwd 501 | awk -F: '$1 != "nobody" { print $1; exit }' || true)"
if [ -z "${HOSTSHIFT_LOGIN_USER}" ]; then
  HOSTSHIFT_LOGIN_USER="$(awk -F: '($3 >= 1000 && $3 < 60000) && $1 != "nobody" && $7 !~ /(nologin|false)$/ { print $1; exit }' /etc/passwd || true)"
fi
HOSTSHIFT_OS_ID=""
if [ -r /etc/os-release ]; then
  . /etc/os-release
  HOSTSHIFT_OS_ID="${ID:-}"
fi
MYSQL_SERVER_PACKAGE="mysql-server"
if [ "${HOSTSHIFT_OS_ID}" = "debian" ]; then
  MYSQL_SERVER_PACKAGE="default-mysql-server"
fi

HOSTSHIFT_CREATED_POLICY_RC_D=0
if [ ! -e /usr/sbin/policy-rc.d ]; then
  printf '#!/bin/sh\nexit 101\n' >/usr/sbin/policy-rc.d
  chmod 755 /usr/sbin/policy-rc.d
  HOSTSHIFT_CREATED_POLICY_RC_D=1
fi
cleanup_policy_rc_d() {
  if [ "${HOSTSHIFT_CREATED_POLICY_RC_D}" = "1" ]; then
    rm -f /usr/sbin/policy-rc.d
  fi
}
trap cleanup_policy_rc_d EXIT

apt-get install -y \
  apache2 \
  nginx \
  "${MYSQL_SERVER_PACKAGE}" \
  postgresql \
  ufw \
  nftables

cleanup_policy_rc_d
trap - EXIT

mkdir -p /srv/hostshift-fixture/public /srv/hostshift-fixture/config /srv/hostshift-fixture/fixtures/mysql /srv/hostshift-fixture/fixtures/postgresql
printf 'ok\n' > /srv/hostshift-fixture/public/health
printf '{"mode":"vm-fixture"}\n' > /srv/hostshift-fixture/config/standalone.json
printf 'APP_ENV=production\nDB_CONNECTION=mysql\n' > /srv/hostshift-fixture/.env
printf 'systemd-fixture-v1\n' > /srv/hostshift-fixture/systemd-marker

cat >/etc/systemd/system/hostshift-fixture-app.service <<'EOF'
[Unit]
Description=HostShift VM fixture application
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/bin/sleep infinity
Restart=always

[Install]
WantedBy=multi-user.target
EOF

cat >/etc/nginx/sites-available/hostshift-fixture.conf <<'EOF'
server {
    listen 80 default_server;
    server_name _;
    root /srv/hostshift-fixture/public;

    location /health {
        try_files /health =404;
    }
}
EOF

ln -sf /etc/nginx/sites-available/hostshift-fixture.conf /etc/nginx/sites-enabled/hostshift-fixture.conf

cat >/etc/apache2/ports.conf <<'EOF'
Listen 8080
EOF

cat >/etc/apache2/sites-available/hostshift-fixture.conf <<'EOF'
<VirtualHost *:8080>
    ServerName localhost
    DocumentRoot /srv/hostshift-fixture/public

    <Directory /srv/hostshift-fixture/public>
        Require all granted
    </Directory>
</VirtualHost>
EOF

a2dissite 000-default.conf || true
a2ensite hostshift-fixture.conf
apache2ctl configtest

systemctl enable ssh nginx apache2 hostshift-fixture-app.service
systemctl enable mysql || systemctl enable mariadb
systemctl enable postgresql || true
systemctl restart ssh nginx apache2 hostshift-fixture-app.service postgresql || true
systemctl restart mysql || systemctl restart mariadb || true

mysql <<'SQL'
CREATE DATABASE IF NOT EXISTS hostshiftvm;
USE hostshiftvm;
CREATE TABLE IF NOT EXISTS pages (
  id INT PRIMARY KEY,
  slug VARCHAR(64) NOT NULL,
  body VARCHAR(255) NOT NULL
);
REPLACE INTO pages (id, slug, body) VALUES
  (1, 'home', 'hostshift home'),
  (2, 'health', 'hostshift health');
SQL

if [ -n "${HOSTSHIFT_LOGIN_USER}" ]; then
  mysql <<SQL
CREATE USER IF NOT EXISTS '${HOSTSHIFT_LOGIN_USER}'@'localhost';
GRANT SELECT, SHOW VIEW, TRIGGER ON hostshiftvm.* TO '${HOSTSHIFT_LOGIN_USER}'@'localhost';
FLUSH PRIVILEGES;
SQL
fi

runuser -u postgres -- psql --tuples-only --no-align --command "SELECT 1 FROM pg_roles WHERE rolname='root'" postgres | grep -qx 1 || runuser -u postgres -- createuser -s root
if [ -n "${HOSTSHIFT_LOGIN_USER}" ]; then
  runuser -u postgres -- psql --tuples-only --no-align --command "SELECT 1 FROM pg_roles WHERE rolname='${HOSTSHIFT_LOGIN_USER}'" postgres | grep -qx 1 || runuser -u postgres -- createuser -s "${HOSTSHIFT_LOGIN_USER}"
fi
runuser -u postgres -- psql --tuples-only --no-align --command "SELECT 1 FROM pg_database WHERE datname='hostshiftpg'" postgres | grep -qx 1 || runuser -u postgres -- createdb -O root hostshiftpg

psql --username root --dbname hostshiftpg <<'SQL'
CREATE TABLE IF NOT EXISTS metrics (
  id integer primary key,
  name text not null
);
INSERT INTO metrics (id, name) VALUES
  (1, 'uptime'),
  (2, 'traffic')
ON CONFLICT (id) DO UPDATE SET name = excluded.name;
SQL

if [ -n "${HOSTSHIFT_LOGIN_USER}" ]; then
  psql --username root --dbname hostshiftpg --command "GRANT CONNECT ON DATABASE hostshiftpg TO \"${HOSTSHIFT_LOGIN_USER}\";"
  psql --username root --dbname hostshiftpg --command "GRANT USAGE ON SCHEMA public TO \"${HOSTSHIFT_LOGIN_USER}\";"
  psql --username root --dbname hostshiftpg --command "GRANT SELECT ON ALL TABLES IN SCHEMA public TO \"${HOSTSHIFT_LOGIN_USER}\";"
fi
