import test from "node:test";
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";

test("docker matrix runner lists required cross-distro pairs", () => {
  const result = spawnSync(process.execPath, ["tests/integration/docker/run-matrix.mjs", "--list"], {
    cwd: process.cwd(),
    encoding: "utf8"
  });
  assert.equal(result.status, 0, result.stderr);
  assert.match(result.stdout, /ubuntu22 -> ubuntu24/);
  assert.match(result.stdout, /ubuntu22 -> debian12/);
  assert.match(result.stdout, /debian12 -> ubuntu22/);
  assert.match(result.stdout, /debian12 -> debian13/);
});

test("docker matrix dry-run documents real immutability checks", () => {
  const result = spawnSync(process.execPath, ["tests/integration/docker/run-matrix.mjs"], {
    cwd: process.cwd(),
    encoding: "utf8"
  });
  assert.equal(result.status, 0, result.stderr);
  assert.match(result.stdout, /source immutability checks/);
});

test("docker matrix runner can filter a single pair", () => {
  const result = spawnSync(process.execPath, ["tests/integration/docker/run-matrix.mjs", "--list", "--pair", "ubuntu22->debian12"], {
    cwd: process.cwd(),
    encoding: "utf8"
  });
  assert.equal(result.status, 0, result.stderr);
  assert.equal(result.stdout.trim(), "ubuntu22 -> debian12");
});

test("docker matrix runner lists unique fixture base images", () => {
  const result = spawnSync(process.execPath, ["tests/integration/docker/run-matrix.mjs", "--list-images"], {
    cwd: process.cwd(),
    encoding: "utf8"
  });
  assert.equal(result.status, 0, result.stderr);
  assert.deepEqual(result.stdout.trim().split(/\r?\n/), [
    "debian:12",
    "debian:13",
    "ubuntu:22.04",
    "ubuntu:24.04",
    "ubuntu:25.10"
  ]);
});
