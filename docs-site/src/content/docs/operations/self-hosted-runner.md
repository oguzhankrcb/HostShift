---
title: Self-Hosted VM Runner
description: Offline-by-default macOS runner for the VM apply gate.
---

The real VM apply gate uses a self-hosted macOS runner because GitHub hosted macOS runners do not reliably boot nested Lima VMs.

## Security Rule

Do not install the VM runner as a macOS service.

Keep the runner offline by default. Start it manually only when you intentionally want to run the VM apply gate.

The runner must have the `hostshift-vm` label:

```yaml
runs-on: [self-hosted, macOS, hostshift-vm]
```

## Start The Runner

```bash
cd ~/actions-runner-hostshift
./run.sh
```

Stop it with `Ctrl-C` after the workflow finishes.

Do not run:

```bash
sudo ./svc.sh install
sudo ./svc.sh start
```

## Run The Gate

Use the `VM E2E Apply` workflow, or the `CI` workflow with `run_vm_apply` checked.
