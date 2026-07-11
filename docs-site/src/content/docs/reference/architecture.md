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

Every workload type must resolve through the production workload registry. An adapter plan result owns:

- blockers
- required target capabilities and package preparation
- target actions across prepare, verify, and cutover phases
- typed read-only source-to-target streams
- target rollback metadata attached to actions and streams

Source discovery first normalizes allowlisted host facts into profile workloads. The planner then requires every discovered or manually reviewed workload type to have a registered adapter. An unregistered type creates a blocker instead of being silently skipped.

`BuildWithRegistry` is the internal extension boundary used to test and add workload adapters without adding another planner dispatch path.

## Action Model

Every planned operation is represented as an action:

```text
Action{id, phase, hostRole, impact, command, preconditions, rollback}
```

Source actions are limited to facts and read-only exports. Target actions may write only during apply phases and only after the plan is reviewed.

## State And Audit

Runs atomically write resumable state after every completed action or stream and append JSONL audit events. State includes a phase plan fingerprint and tracks completed, failed, and uncertain IDs. `resume` rebuilds the plan, rejects fingerprint drift, skips completed work, and requires explicit confirmation before retrying a potentially partial operation.
