---
title: Plans, State, And Audit
description: Action model, stream model, blockers, warnings, run state, and audit logs.
---

HostShift separates planning from execution. `plan`, `prepare`, `sync`, and `verify` expose the same source-safety metadata so automation can refuse risky work before apply.

## Plan Shape

`hostshift plan --json` returns:

```json
{
  "profile": "example",
  "sourcePolicy": "strict-read-only",
  "sourceWillBeModified": false,
  "actions": [],
  "streams": [],
  "blockers": [],
  "warnings": []
}
```

## Actions

Actions are local, source, or target commands:

```text
Action{id, phase, hostRole, impact, command, preconditions, rollback}
```

Valid phases:

- `discover`
- `plan`
- `prepare`
- `sync`
- `verify`
- `cutover`
- `rollback`

Valid host roles:

- `source`
- `target`
- `local`

Valid impacts:

- `read-only`
- `write`
- `service`
- `network`

Any source action must have `impact: read-only`. Plan validation fails otherwise.

## Streams

Streams pipe a source read command into a target write command:

```text
StreamAction{id, phase, sourceCommand, targetCommand, preconditions, rollback}
```

Examples:

- `tar --create` to `tar --extract`
- `docker image save` to `docker image load`
- `mysqldump` to `mysql`
- `pg_dump` to `pg_restore`
- existing Docker volume snapshot tar `cat` to target `tar --extract`

The source command is validated by the source command guard. The target command is validated by the target command guard and may be sudo-wrapped only when `HOSTSHIFT_TARGET_SUDO=1`.

## Blockers

Common blockers:

- profile is not approved
- `sourcePolicy` is not `strict-read-only`
- target SSH alias is missing
- target platform is unknown
- target platform is EOL or unsupported
- package capability cannot be mapped to a target package
- a discovered Docker named volume has no reviewed strategy
- a Docker volume `snapshot` strategy has no existing `snapshotPath`

Apply should not run while blockers are present.

## Warnings

Warnings call out risk that does not necessarily block apply:

- source platform is EOL but still read-only
- cross-distribution migration requires compatibility checks

Warnings should be handled through explicit checks in the profile.

## State Directory

Apply phases can write state:

```bash
hostshift sync --profile profile.yaml --apply --state-dir .hostshift --run-id sync-001 --json
```

State path:

```text
<state-dir>/runs/<run-id>/state.json
```

A per-run OS file lock prevents two dry-run/apply/resume processes from writing or executing the same run concurrently. `status` and resume preview can still read the atomically replaced state document.

If `--state-dir` is omitted, HostShift uses:

1. `HOSTSHIFT_STATE_DIR`
2. the OS user config directory under `hostshift`
3. `.hostshift` as fallback

If `--run-id` is omitted, phase commands generate a collision-resistant ID and return it as `runId` in their output so the saved state can be addressed later.

## Audit Log

Audit events are appended as JSONL:

```text
<state-dir>/runs/<run-id>/audit.jsonl
```

Each event includes:

- `time`
- `runId`
- `phase`
- `action`
- optional `message`

The state document includes:

- `planHash`: SHA-256 fingerprint of the phase profile, hosts, blockers, actions, and streams
- `status`: `dry-run`, `blocked`, `running`, `failed`, or `completed`
- `completed`: action and stream IDs durably completed by apply
- `failedAction` and redacted `lastError` when execution returned an error
- `uncertainAction` while a remote operation is running or may have partially completed

## Status And Resume

Use `status` to read state:

```bash
hostshift status --state-dir .hostshift --run-id sync-001 --json
```

Use `resume` without `--apply` to preview completed and pending steps without changing the saved state:

```bash
hostshift resume --profile migration.profile.yaml --state-dir .hostshift --run-id sync-001 --json
```

Use `--apply` to continue the same phase. HostShift rebuilds the plan and requires the saved fingerprint to match before it skips completed IDs and executes pending IDs.

```bash
hostshift resume --profile migration.profile.yaml --state-dir .hostshift --run-id sync-001 --apply --json
```

If `failedAction` or `uncertainAction` is present, inspect the target and pass the exact ID with `--retry-failed`. HostShift never silently retries a potentially partial remote operation.
