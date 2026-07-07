---
title: Contributing
description: How to contribute safely.
---

Contributions should preserve the source-read-only invariant.

## Local Checks

```bash
make test-go
make build
make test-integration-docker
make test-e2e-vm
```

Use real Docker and VM gates for changes that affect workload behavior, source safety, package planning, firewall behavior, systemd behavior, or migration verification.

## Review Expectations

Changes should include focused tests when they affect:

- source command safety
- profile schema behavior
- planner output
- target package mapping
- workload streams
- verification checks
- Docker or VM test fixtures

Do not add source-side writes as convenience shortcuts.
