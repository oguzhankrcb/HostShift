#!/usr/bin/env bash
set -euo pipefail

mkdir -p /srv/hostshift-target
systemctl enable ssh
systemctl enable postgresql || true
systemctl restart ssh postgresql || true

if command -v psql >/dev/null 2>&1; then
  runuser -u postgres -- psql --tuples-only --no-align --command "SELECT 1 FROM pg_roles WHERE rolname='root'" postgres | grep -qx 1 || runuser -u postgres -- createuser -s root
  runuser -u postgres -- psql --tuples-only --no-align --command "SELECT 1 FROM pg_database WHERE datname='hostshiftpg'" postgres | grep -qx 1 || runuser -u postgres -- createdb -O root hostshiftpg
fi

if command -v nft >/dev/null 2>&1; then
  systemctl enable nftables || true
  systemctl restart nftables || true
  nft add table inet hostshift 2>/dev/null || true
  nft 'add chain inet hostshift input { type filter hook input priority 0; policy accept; }' 2>/dev/null || true
  nft add rule inet hostshift input tcp dport 3306 accept 2>/dev/null || true
  nft list ruleset > /etc/nftables.conf
fi
