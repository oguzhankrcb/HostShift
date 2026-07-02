---
title: Security
description: Security policy summary.
---

HostShift is pre-1.0 software. Report suspected vulnerabilities privately before public disclosure.

## Sensitive Data

Do not commit:

- production profiles
- `.env` files
- SSH private keys
- generated `ssh_config`
- PEM/key material
- run state containing customer details

The repository `.gitignore` excludes common secret-bearing files and generated artifacts.

## Credentials

Profiles should reference credentials through environment variable names, for example:

```yaml
sourcePasswordEnv: SRC_MYSQL_PWD
targetPasswordEnv: DST_MYSQL_PWD
```

Do not store passwords directly in profiles.

## Runner Safety

The self-hosted VM runner is offline by default and manually started only for release validation. It is not installed as a service.
