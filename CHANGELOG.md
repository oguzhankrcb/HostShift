# Changelog

## Unreleased

- Added Docker named-volume discovery and explicit snapshot, disposable, database-backed, and external migration strategies with full Docker matrix checksum coverage.
- Added executable phase resume with atomic checkpoints, plan fingerprints, completed-step skipping, uncertain-action retry confirmation, per-run locking, and MCP status/resume previews.
- Expanded real VM migration coverage with Apache vhosts, a standalone systemd application, confirmed target-only cutover, reboot persistence, and broader source checksums.
- Added typed validation for read-only source tar streams so safe filenames such as `/etc/logrotate.d/apt` do not trigger package-manager false positives while option injection and traversal remain blocked.
- Added direct source service PID/start-time immutability comparisons to Docker and VM migration gates.
- Enforced a successful self-hosted real VM apply run for the exact commit before the Release workflow can publish artifacts.
- Replaced the abbreviated license notice with the complete canonical Apache License 2.0 text for public distribution.
- Added full-history Gitleaks checks to CI and release gates before making the repository public.
- Added Dependabot coverage and safety-focused issue, feature, and pull-request templates for public maintenance.

## v0.3.0 - Pending

- Rebuilt HostShift as a Go-first CLI with the Node v0.2 behavior kept as a compatibility reference.
- Added strict read-only-source planning, source command allowlisting, and mutation rejection tests.
- Added profile v2 YAML support with JSON Schema validation and profile migration.
- Added Ubuntu/Debian platform adapters, target package planning, OpenSSH runner quoting, audit state, and resumable run support.
- Added Docker Compose, standalone container, file-set, MySQL/MariaDB, PostgreSQL, Nginx, Laravel-style DB, SSH, UFW, and nftables migration/verification coverage.
- Added real Docker migration matrix coverage for Ubuntu/Debian cross-distro pairs.
- Added Lima-based VM e2e scaffolding and apply workflow for systemd, package, firewall, DB, HTTP, boot persistence, and source immutability checks.
- Added GoReleaser config, GitHub Actions scaffolding, checksum generation, and SPDX SBOM generation.
