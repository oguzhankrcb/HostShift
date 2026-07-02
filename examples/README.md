# HostShift Examples

These profiles are safe templates, not drop-in production plans. Keep `approved: false` until you review every workload, target path, firewall rule, and verification check.

## Profiles

- `profile.yaml`: legacy v1 JSON-compatible profile used by compatibility tests.
- `profile.v2.yaml`: compact profile v2 example covering Docker Compose, standalone containers, file sets, MySQL, firewall, SSH keepalive, MySQL bind-address, HTTP checks, and Laravel database checks.
- `web-stack-v2.yaml`: fuller public example for an Ubuntu 22.04 to Debian 12 web stack migration.

## Workflow

```bash
hostshift plan --profile examples/web-stack-v2.yaml --json
hostshift prepare --profile examples/web-stack-v2.yaml --json
hostshift sync --profile examples/web-stack-v2.yaml --json
hostshift verify --profile examples/web-stack-v2.yaml --json
```

Only add `--apply` after the dry-run plan is clean and the target host is disposable or otherwise ready for writes.
