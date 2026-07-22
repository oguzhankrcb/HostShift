# Docker Migration Matrix

This directory contains the first integration-test shape for real source-to-target migrations over SSH.

Docker is useful for repeatable workload tests, but it does not fully model systemd, kernel firewall behavior, boot ordering, or provider images. VM tests under `tests/e2e/vm` cover those later.

## Matrix

- `ubuntu22 -> ubuntu22, ubuntu24, ubuntu25, debian12`
- `debian12 -> ubuntu22, ubuntu24, ubuntu25, debian12, debian13`

Each source fixture must expose:

- SSH
- Docker Compose workload
- standalone container workload
- Docker named-volume snapshot fixture
- MySQL/MariaDB data
- PostgreSQL data
- Nginx or Apache vhost
- Laravel-like database connectivity check
- firewall and SSH configuration fixtures

Each test asserts:

- target workloads run
- HTTP health checks pass
- MySQL row counts and checksums match
- PostgreSQL row counts and checksums match
- expected config values exist on target
- source immutability markers are unchanged and MariaDB, PostgreSQL, and SSH keep the same PID/start time

Run a safe dry-run:

```bash
make test-integration-docker
```

Run real fixture checks for every pair:

```bash
HOSTSHIFT_RUN_DOCKER_MATRIX=1 make test-integration-docker
```

List or pre-pull required base images:

```bash
bash tests/integration/docker/run-matrix.sh --list-images
make docker-pull-fixtures
```

Real mode pre-pulls the selected fixture base images before building source/target containers unless `HOSTSHIFT_DOCKER_SKIP_PREPULL=1` is set. Package installation layers use the official Kernel.org Ubuntu archive mirror, source and target images build serially to avoid mirror contention, and layers are reused across matrix pairs while each pair still receives fresh containers and SSH credentials. `HOSTSHIFT_DOCKER_PULL_TIMEOUT_MS`, `HOSTSHIFT_DOCKER_BUILD_TIMEOUT_MS`, and `HOSTSHIFT_DOCKER_COMMAND_TIMEOUT_MS` can be used to shorten or lengthen diagnostics in local and CI environments.

Real mode currently does the following for each matrix pair:

- renders Docker Compose config
- builds and boots source and target fixtures
- verifies source immutability fixture checksums
- generates temporary SSH credentials and aliases for the containers
- runs `hostshift discover` against the source fixture over SSH
- runs `hostshift plan`, `prepare`, `sync`, and `verify` in dry-run mode against a generated profile
- runs a real `hostshift sync --apply` smoke profile for each matrix pair, including MySQL/PostgreSQL restore, Redis snapshot streaming, and existing Docker volume snapshot extraction
- runs a real `hostshift verify --apply` smoke profile for each matrix pair, including HTTP and Laravel-style DB checks
- verifies copied target fixture artifacts and source-vs-target checksums for selected files
- verifies target HTTP health response, Laravel-style DB connectivity, MySQL/PostgreSQL row-count/checksum parity, Redis snapshot checksum parity, and Docker volume data checksum parity
- re-checks source immutability markers and source service PID/start-time snapshots after apply

Docker Engine or Docker Desktop must be running for real mode. The runner now fails early if the Docker daemon is unavailable or if required base images cannot be pulled through the daemon.

The Docker runner currently validates real SSH discovery, dry-run orchestration across web service config workloads, a file-set plus MySQL, PostgreSQL, Redis snapshot, and Docker volume snapshot `sync --apply` smoke path, and `verify --apply` checks for HTTP plus Laravel-style DB connectivity. The source fixture also carries Caddy, PHP-FPM, Supervisor, Fail2ban, Memcached, RabbitMQ, Certbot, and Logrotate config files so config transfer and discovery candidates are checked in the matrix.
