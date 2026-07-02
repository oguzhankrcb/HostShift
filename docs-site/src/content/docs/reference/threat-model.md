---
title: Threat Model
description: Threats HostShift is designed to reduce.
---

HostShift is designed around a high-impact failure mode: accidentally damaging a live source server while migrating it.

## Protected Asset

The primary protected asset is the source server's running state, including:

- files
- services
- firewall rules
- database state
- container state
- machine identity
- availability

## Main Controls

- source command allowlisting
- mutation rejection for source commands
- source snapshot checks in VM tests
- blockers for unsafe online reads
- explicit `--apply` for target writes
- redaction of secret-bearing commands
- target rollback metadata

## Residual Risks

HostShift cannot guarantee application-level consistency for every live workload. If a workload cannot be safely read online, the correct behavior is to block and require an operator decision.

Cloud provider networking, DNS, snapshots, and billing resources are intentionally outside the core threat model.
