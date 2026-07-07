---
title: Quick Start
description: Run a first dry-run migration rehearsal.
---

Start with a dry-run profile and SSH aliases that already work from the machine running HostShift.

```bash
ssh old-server true
ssh new-server true
```

Run the first discovery and plan:

```bash
hostshift doctor --source old-server --target new-server --json
hostshift discover --source old-server --name rehearsal --profile rehearsal.profile.yaml --json
hostshift plan --profile rehearsal.profile.yaml --target new-server --json
hostshift explain --profile rehearsal.profile.yaml --target new-server --json
```

Run dry-run phases:

```bash
hostshift prepare --profile rehearsal.profile.yaml --target new-server --json
hostshift sync --profile rehearsal.profile.yaml --target new-server --json
hostshift verify --profile rehearsal.profile.yaml --target new-server --json
```

## Apply Discipline

Only use `--apply` after:

- the profile is reviewed and approved
- blockers are resolved
- target-side writes are expected
- database credentials are supplied through environment variables
- rollback notes are understood

```bash
hostshift prepare --profile rehearsal.profile.yaml --target new-server --apply --json
hostshift sync --profile rehearsal.profile.yaml --target new-server --apply --json
hostshift verify --profile rehearsal.profile.yaml --target new-server --apply --json
```

The source server remains read-only in both dry-run and apply modes.
