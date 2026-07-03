---
title: Overview
description: What HostShift does and what it deliberately does not do.
---

HostShift migrates server workloads by separating the work into discovery, planning, target preparation, synchronization, verification, and release/cutover support.

## Design Goals

- Preserve the source server exactly as it is.
- Make every planned action inspectable before apply.
- Treat unsafe online reads as blockers instead of silently skipping them.
- Keep cloud and DNS automation outside the core.
- Support realistic Ubuntu and Debian version differences.
- Verify results with Docker and VM test matrices, not only unit tests.

## Non-Goals

HostShift does not:

- edit source host files
- stop source services
- install source packages
- configure cloud firewalls or DNS records
- invent package names for unknown platforms
- migrate machine identity files such as SSH host keys or `/etc/machine-id`

## Main Workflow

```bash
hostshift doctor --source old-server --target new-server --json
hostshift discover --source old-server --name migration --profile migration.profile.yaml --json
hostshift plan --profile migration.profile.yaml --target new-server --json
hostshift prepare --profile migration.profile.yaml --target new-server --json
hostshift sync --profile migration.profile.yaml --target new-server --json
hostshift verify --profile migration.profile.yaml --target new-server --json
```

`prepare`, `sync`, and `verify` default to dry-run mode. Add `--apply` only after reviewing blockers and actions.

For exact flags and output shapes, see [CLI Reference](/reference/cli/). For profile fields, see [Profile v2 Reference](/reference/profile-v2/).
