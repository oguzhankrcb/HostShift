# Changelog

## Unreleased

- Rebuilt HostShift as a Go-first CLI with the Node v0.2 behavior kept as a compatibility reference.
- Added strict read-only-source planning, source command allowlisting, and mutation rejection tests.
- Added profile v2 YAML support with JSON Schema validation and profile migration.
- Added Ubuntu/Debian platform adapters, target package planning, OpenSSH runner quoting, audit state, and resumable run support.
- Added Docker Compose, standalone container, file-set, MySQL/MariaDB, PostgreSQL, Nginx, Laravel-style DB, SSH, UFW, and nftables migration/verification coverage.
- Added real Docker migration matrix coverage for Ubuntu/Debian cross-distro pairs.
- Added Lima-based VM e2e scaffolding and apply workflow for systemd, package, firewall, DB, HTTP, boot persistence, and source immutability checks.
- Added GoReleaser config, GitHub Actions scaffolding, checksum generation, and SPDX SBOM generation.
