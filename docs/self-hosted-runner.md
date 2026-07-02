# Self-Hosted VM Runner

HostShift uses a self-hosted macOS runner only for the real Lima VM apply gate. GitHub hosted macOS runners run preflight checks, but they do not reliably boot nested VMs.

## Security Rule

Do not install the VM runner as a macOS service.

Keep the runner offline by default. Start it manually only when you intentionally want to run the VM apply gate. This prevents repository workflows from running on the local Mac without an explicit local action.

The runner must be registered with the `hostshift-vm` label. Workflows require:

```yaml
runs-on: [self-hosted, macOS, hostshift-vm]
```

## Local Runner

Current expected local path:

```bash
~/actions-runner-hostshift
```

Required local tools:

```bash
brew install lima
xcode-select --install
```

Start the runner manually when needed:

```bash
cd ~/actions-runner-hostshift
./run.sh
```

Stop it with `Ctrl-C` after the VM apply workflow finishes.

Do not run:

```bash
sudo ./svc.sh install
sudo ./svc.sh start
```

## Running The Gate

Use either workflow:

- `VM E2E Apply`
- `CI` with `run_vm_apply` checked

The standalone `VM E2E Apply` workflow is preferred for release validation because it only runs the real VM apply gate.
