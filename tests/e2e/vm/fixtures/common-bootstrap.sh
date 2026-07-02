#!/usr/bin/env bash
set -euo pipefail

export DEBIAN_FRONTEND=noninteractive

apt-get update
apt-get install -y \
  ca-certificates \
  curl \
  nftables \
  openssh-server \
  rsync \
  sudo \
  tar

mkdir -p /var/run/sshd
