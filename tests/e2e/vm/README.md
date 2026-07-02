# VM E2E Tests

VM tests cover behavior Docker cannot model reliably:

- systemd unit installation and service state
- real firewall backends
- package installation on fresh provider-like images
- boot persistence
- kernel networking behavior

## Current State

The VM layer is now executable:

- `matrix.yaml` defines the first Ubuntu/Debian cross-distro pairs
- `run-vm-e2e.mjs` validates the matrix and renders per-pair workspaces
- `providers/lima/instance-plan.json.tmpl` emits provider-specific plan manifests
- generated `source.lima.yaml` and `target.lima.yaml` files base on official Lima distro templates
- `fixtures/*.sh` define baseline source and target bootstrap intent
- `run-vm-e2e.sh` adds a shell entrypoint and provider binary checks

Run it with:

```bash
make test-e2e-vm
HOSTSHIFT_RUN_VM_E2E=1 make test-e2e-vm
make build
HOSTSHIFT_RUN_VM_E2E=1 bash tests/e2e/vm/run-vm-e2e.sh --pair ubuntu22->debian12 --apply
```

Default mode is dry-run only. It renders artifacts such as:

- `source.plan.json`
- `target.plan.json`
- `pair.json`
- `source.lima.yaml`
- `target.lima.yaml`
- `commands.json`

`HOSTSHIFT_RUN_VM_E2E=1` adds provider preflight. For Lima this currently means validating that `limactl` exists and responds correctly.

`--apply` now performs the first real provider workflow. The runner uses `dist/hostshift` when it exists and falls back to the Node reference CLI only when the Go binary has not been built. Override with `HOSTSHIFT_VM_HOSTSHIFT_BIN=/path/to/hostshift` when testing a specific binary.

- validates the rendered Lima templates
- boots source and target instances
- queries connection data with `limactl show-ssh --format=options`
- writes a temporary combined `ssh_config`
- captures a source-side checksum snapshot for fixture files
- runs live HostShift `discover`, `plan`, `prepare`, `sync`, and `verify` against the booted aliases
- verifies copied files, Nginx config reload plus HTTP health, MySQL row/checksum parity, PostgreSQL row/checksum parity, systemd service state, and planned UFW/nftables rule state
- restarts the target VM and runs HostShift `verify` again to catch boot-persistence regressions
- checks that the source checksum snapshot did not change
- stops and deletes the instances unless `HOSTSHIFT_VM_KEEP_INSTANCES=1`

Generated Lima templates intentionally ignore automatic guest port forwarding except for Lima-managed SSH. HostShift verifies HTTP and database behavior through SSH-targeted checks, so exposing guest port 80, 3306, 5432, or 5355 to the host only creates noisy and environment-dependent port conflicts.

Set `HOSTSHIFT_VM_LIMA_VM_TYPE=qemu` to force Lima's QEMU driver. GitHub hosted macOS runners use this because the default VZ driver may fail before the guest boot log is available; those jobs install both `lima` and `qemu`.

Timeout controls:

- `HOSTSHIFT_VM_COMMAND_TIMEOUT_MS` defaults to 15 minutes for generic commands
- `HOSTSHIFT_VM_LIMACTL_TIMEOUT_MS` defaults to 20 minutes for Lima lifecycle commands
- `HOSTSHIFT_VM_HOSTSHIFT_TIMEOUT_MS` defaults to 10 minutes for HostShift phases
- `HOSTSHIFT_VM_SSH_TIMEOUT_MS` defaults to 2 minutes for direct SSH snapshot commands

## Safety Model

The source VM remains read-only by contract:

- every generated pair manifest writes `sourcePolicy: strict-read-only`
- source bootstrap scripts model representative fixtures, not migration-side migration writes
- the current `--apply` path still limits itself to fixture bootstrap, SSH assembly, source snapshot comparison, and HostShift profile-driven `prepare/sync/verify` fixture validation

## Release Role

The Docker matrix remains the fast inner loop. VM runs are the release gate for behavior Docker cannot model: package installation, systemd, real firewall state, boot persistence, and provider-image differences.

Future fixture expansion should add Apache and standalone systemd application coverage, but the current apply workflow already verifies package install, systemd service state, firewall rule state, HTTP health, database parity, boot persistence, and source immutability against booted VMs.
