# Security Policy

## Source Safety

HostShift must never mutate the source server. Reports that show source-side writes, service management, package installation, firewall changes, key changes, snapshots, maintenance mode, or arbitrary shell execution are treated as security bugs.

## Reporting

Please open a private security advisory on GitHub once the repository is published. Until then, report issues directly to the maintainer.

## Secrets

Do not include `.env` files, database credentials, SSH private keys, Docker inspect output, or production profile files in public issues.
