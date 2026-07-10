# HostShift Architecture

HostShift is built around typed plans.

## Core Interfaces

- `PlatformAdapter` detects the OS, support status, package manager, firewall backends, and service manager.
- Package installation is capability based, for example `docker-runtime` or `postgresql-client`, then mapped by the platform adapter.
- `WorkloadAdapter` owns discovery, planning, sync, target preparation, verification, and target rollback for one workload type.
- `Action` is the executable unit. Every action declares phase, host role, impact, command, preconditions, and rollback metadata.

## Execution Boundary

The source host can only run read-only facts and typed read-only exports. Target actions may mutate the target after profile approval and `--apply`.

## State

Runs store `state.json` and `audit.jsonl` under the HostShift state directory. State writes are atomic and record a phase plan fingerprint, status, completed IDs, and any failed or uncertain action. The audit log is append-only and redacted before writing.

Resume rebuilds the plan from the reviewed profile and refuses execution when the fingerprint differs. Completed IDs are skipped. A failed or interrupted action is treated as potentially partial and requires an exact operator `--retry-failed <action-id>` confirmation before it can run again. A non-blocking OS file lock permits only one state-writing process per run ID.

## Execution

The executor filters actions by phase. Dry-run mode records state but does not run remote commands. Apply mode refuses any plan with blockers, routes source actions through source-only command validation, and routes target actions through target command validation.

Stream actions model source stdout flowing into target stdin. They are used for file-set tar streaming, Docker image streaming, and database dump/restore flows. The source side is always validated as read-only before execution.

Target first-install configuration is represented as target-only prepare actions. UFW rules, sshd keepalive drop-ins, and MySQL bind-address drop-ins are validated from typed profile fields and include rollback metadata. Source-side firewall, SSH, and MySQL configuration remains fact-only.

Database passwords are represented as environment variable names such as `sourcePasswordEnv`, never literal secret values. Plan JSON redacts password arguments before printing.

Verification checks are typed target-side read operations. HTTP checks use `curl`; Laravel database checks execute a fixed `DB::connection()->getPdo()` probe inside the selected target container.
