---
title: Source Discovery
description: Read-only source fact collection and generated profile behavior.
---

Source discovery is an allowlisted read-only operation. HostShift treats the source as an observation endpoint and never writes to it.

## Fact Execution

`hostshift discover` runs each fact through the source command guard. Required facts must succeed. Optional facts can fail and still be reported in the JSON output.

```bash
hostshift discover --source old-server --name migration --profile migration.profile.yaml --json
```

## Required Facts

Required facts currently include:

- `osRelease`: `cat /etc/os-release`
- `architecture`: `uname -m`
- `hostname`: `hostname`
- `disk`: `df -Pk`
- `memory`: `cat /proc/meminfo`
- `packages`: `dpkg-query -W -f=${binary:Package}\t${Version}\n`
- `mounts`: `findmnt --json --real`
- `users`: `getent passwd`
- `groups`: `getent group`

If any required fact fails, discovery fails instead of generating a misleading profile.

## Optional Facts

Optional facts provide workload hints:

- `enabledServices`
- `runningServices`
- `listeners`
- `ufwStatus`
- `nftRuleset`
- `sshdEffectiveConfig`
- `sshdConfig`
- `mysqlServerConfig`
- `mysqlDatabases`
- `postgresDatabases`
- `nginxConfigDump`
- `apacheConfigDump`
- `letsEncryptFiles`
- `cron`
- `dockerVersion`
- `dockerComposeProjects`
- `dockerContainers`
- `dockerNetworks`

Optional failures are visible in output and should be reviewed before migration.

## Generated Profile

Discovery writes a profile with:

- `schemaVersion: 2`
- `source.ssh` set to the discovered alias
- `sourcePolicy: strict-read-only`
- `approved: false`
- `platforms.source` populated from `/etc/os-release` when available
- empty workloads and checks

Operators must fill in target, workloads, checks, and approval after reviewing the discovery output.

## Source Immutability

Discovery does not use:

- `sudo`
- package installation
- service management
- file writes
- snapshot creation
- maintenance mode
- firewall changes

The same source command guard also protects source-side sync streams.

