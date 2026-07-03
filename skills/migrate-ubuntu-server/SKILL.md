---
name: migrate-ubuntu-server
description: Legacy alias for HostShift server migrations. Use migrate-server for new Ubuntu or Debian migration work.
---

# Migrate Ubuntu Server

This is a compatibility alias for older HostShift prompts.

For new work, follow the distro-neutral `migrate-server` workflow. HostShift no longer requires source and target Ubuntu versions to match, and it also supports Debian targets through platform compatibility checks.

Keep the same source-safety invariant: the source server is read-only, and all mutations must be target-side actions planned and executed by the `hostshift` CLI.
