---
title: Profiles
description: Profile v2 structure and review flow.
---

Profile v2 is YAML. It describes the source, target, platform expectations, target-only first-install settings, workloads, checks, and approval state.

```yaml
schemaVersion: 2
name: ubuntu22-to-debian12-web-stack
source:
  ssh: source-web
target:
  ssh: target-web
sourcePolicy: strict-read-only
platforms:
  source: ubuntu:22.04
  target: debian:12
approved: false
```

## Required Review

Keep `approved: false` until you review:

- every workload path
- every target package action
- database stream credentials
- firewall rules
- SSH and MySQL target drop-ins
- verification checks
- blockers and warnings

## v1 Migration

Legacy v1 profiles are still readable. Convert them with:

```bash
hostshift profile migrate --input examples/profile.yaml --output profile.v2.yaml
```

## Secrets

Profiles reference secret environment variables instead of embedding passwords:

```yaml
workloads:
  - type: mysql
    name: customer_app
    data:
      sourcePasswordEnv: SRC_MYSQL_PWD
      targetPasswordEnv: DST_MYSQL_PWD
```
