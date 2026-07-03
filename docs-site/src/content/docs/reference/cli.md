---
title: CLI Reference
description: HostShift command reference for migration, safety, state, and profile operations.
---

The primary binary is `hostshift`. Migration behavior lives in the Go CLI; there is no Node migration runtime or compatibility entrypoint.

All commands that can mutate the target default to dry-run mode unless `--apply` is passed.

## Global Behavior

HostShift uses system OpenSSH. Set `HOSTSHIFT_SSH_CONFIG` when tests or automation need a temporary SSH config:

```bash
HOSTSHIFT_SSH_CONFIG=/tmp/hostshift-ssh-config hostshift plan --profile profile.yaml --target new-server --json
```

Target commands can be wrapped with non-interactive sudo by setting `HOSTSHIFT_TARGET_SUDO=1`. Source commands are never sudo-wrapped by HostShift.

## version

Prints the CLI version.

```bash
hostshift version
```

## doctor

Validates source and target SSH aliases and prints the active source-safety contract.

```bash
hostshift doctor --source old-server --target new-server --json
```

Important output fields:

- `sourceWillBeModified: false`
- `sourcePolicy: strict-read-only`
- `version`
- `source`
- `target`

## discover

Reads allowlisted facts from the source and writes a v2 profile with safe workload candidates.

```bash
hostshift discover \
  --source old-server \
  --name customer-migration \
  --profile customer-migration.profile.yaml \
  --json
```

Required flags:

- `--source`: SSH alias for the source.
- `--name`: profile name.

Optional flags:

- `--profile`: output path. Defaults to `<name>.profile.yaml`.
- `--json`: machine-readable output.

`discover` fails if a required fact cannot be read. Optional facts are preserved in output with their error so operators can decide whether they matter for the migration. Generated workload candidates still require operator review before `approved: true`.

## plan

Builds the action and stream plan from a profile.

```bash
hostshift plan --profile examples/web-stack-v2.yaml --target new-server --json
```

Required flags:

- `--profile`: v1 or v2 profile path.

Optional flags:

- `--target`: override the target SSH alias in the profile.
- `--json`: machine-readable output.

The plan contains:

- `actions`: local or target commands grouped by phase.
- `streams`: source-to-target data streams for file, image, and database movement.
- `blockers`: conditions that prevent apply.
- `warnings`: non-blocking risks, for example cross-distribution compatibility warnings.
- `sourceWillBeModified: false`.

## prepare

Plans or applies target preparation actions.

```bash
hostshift prepare --profile profile.yaml --target new-server --json
hostshift prepare --profile profile.yaml --target new-server --apply --state-dir .hostshift --run-id prep-001 --json
```

Typical prepare actions:

- install target package capabilities through the platform adapter
- write target UFW rules
- write target OpenSSH keepalive drop-ins
- write target MySQL bind-address drop-ins
- validate Docker Compose configuration
- disable the default Nginx site when Nginx config is migrated

## sync

Plans or applies data streams.

```bash
hostshift sync --profile profile.yaml --target new-server --json
hostshift sync --profile profile.yaml --target new-server --apply --state-dir .hostshift --run-id sync-001 --json
```

Sync streams are validated source read commands piped into validated target write commands. Examples:

- `tar --create` into `tar --extract`
- `docker image save` into `docker image load`
- `mysqldump` into `mysql`
- `pg_dump` into `pg_restore`

## verify

Plans or applies target-side verification checks.

```bash
hostshift verify --profile profile.yaml --target new-server --json
hostshift verify --profile profile.yaml --target new-server --apply --state-dir .hostshift --run-id verify-001 --json
```

Verification actions run only on the target. They cover HTTP health, Laravel DB connectivity, file assertions, database scalar assertions, service status, firewall rule checks, and Nginx config validation.

## cutover

Plans or applies target-only cutover actions.

```bash
hostshift cutover --profile profile.yaml --target new-server --json
hostshift cutover --profile profile.yaml --target new-server --apply --confirm START-MIGRATION --state-dir .hostshift --run-id cutover-001 --json
```

Dry-run output includes:

- `confirmationCode`
- `sourceWillBeModified: false`
- target-only cutover actions such as `docker compose up -d --build` and standalone `docker run`

Apply refuses blockers and requires the exact confirmation code. DNS remains manual.

## rollback

Reports rollback guidance and target rollback metadata.

```bash
hostshift rollback --profile profile.yaml --json
```

Rollback output always states `sourceChanged: false` because HostShift never mutates the source. Automatic rollback is intentionally disabled; operators should keep DNS on the source and inspect target-side rollback metadata before stopping target services.

## profile migrate

Reads a v1 profile and writes a v2 profile.

```bash
hostshift profile migrate --input examples/profile.yaml --output profile.v2.yaml
```

The migration maps v1 `composeProjects`, `standaloneContainers`, `fileSets`, `databases`, `healthChecks`, and `applicationChecks` into v2 `workloads` and `checks`.

## status

Reads saved run state.

```bash
hostshift status --state-dir .hostshift --run-id sync-001 --json
```

State lives at:

```text
<state-dir>/runs/<run-id>/state.json
```

If `--state-dir` is omitted, HostShift uses `HOSTSHIFT_STATE_DIR` or the OS user config directory.

## resume

Loads a run state and reports the phase that can be resumed.

```bash
hostshift resume --state-dir .hostshift --run-id sync-001
```

In the current milestone, `resume` reports resumability metadata; the execution engine does not automatically continue partial apply work yet.

## policy source

Prints the source policy contract.

```bash
hostshift policy source
```

Forbidden source-side behavior includes `sudo`, package installation, service management, file writes, snapshot creation, maintenance mode, and firewall changes.
