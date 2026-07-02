---
title: Architecture
description: HostShift architecture and extension points.
---

HostShift has two major adapter layers.

## PlatformAdapter

The platform adapter abstracts:

- Ubuntu/Debian detection
- lifecycle support
- package manager commands
- systemd behavior
- firewall commands
- common configuration paths

Unknown targets block apply instead of guessing.

## WorkloadAdapter

The workload adapter describes:

- discovery
- planning
- target preparation
- read-only source export
- target import
- verification
- target rollback metadata

## Action Model

Every planned operation is represented as an action:

```text
Action{id, phase, hostRole, impact, command, preconditions, rollback}
```

Source actions are limited to facts and read-only exports. Target actions may write only during apply phases and only after the plan is reviewed.

## State And Audit

Runs write resumable state and JSONL audit events. `status` and `resume` commands are available for interrupted runs.
