---
title: Web Stack Example
description: Ubuntu 22.04 to Debian 12 web workload profile.
---

The public web stack example lives at `examples/web-stack-v2.yaml`.

It models:

- Ubuntu 22.04 source
- Debian 12 target
- Docker Compose application
- standalone Docker container
- application and Nginx file sets
- MySQL and PostgreSQL streams
- UFW rules
- SSH keepalive settings
- MySQL bind-address settings
- HTTP, Laravel DB, Nginx, systemd, file, and firewall checks

Run a dry plan:

```bash
hostshift plan --profile examples/web-stack-v2.yaml --json
hostshift explain --profile examples/web-stack-v2.yaml --json
```

The profile intentionally has `approved: false`, so the plan includes a blocker. This is expected for a public template.

Before using it, replace:

- SSH aliases
- domains
- file paths
- database names
- container names
- firewall networks
- environment variable names

Then review the generated plan before any `--apply`.
