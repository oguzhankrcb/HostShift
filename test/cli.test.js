import test from "node:test";
import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import fs from "node:fs/promises";
import os from "node:os";
import path from "node:path";

const bin = path.resolve("bin/server-migrate.js");
const profile = path.resolve("examples/profile.yaml");

function cli(args) {
  return new Promise((resolve, reject) => {
    const child = spawn(process.execPath, [bin, ...args], { cwd: path.resolve(".") });
    let stdout = "";
    let stderr = "";
    child.stdout.setEncoding("utf8");
    child.stderr.setEncoding("utf8");
    child.stdout.on("data", (chunk) => { stdout += chunk; });
    child.stderr.on("data", (chunk) => { stderr += chunk; });
    child.on("error", reject);
    child.on("close", (code) => resolve({ code, stdout, stderr }));
  });
}

test("plan command reports source read-only contract", async () => {
  const result = await cli(["plan", "--profile", profile, "--json"]);
  assert.equal(result.code, 0, result.stderr);
  const body = JSON.parse(result.stdout);
  assert.equal(body.sourcePolicy, "strict-read-only");
  assert.equal(body.sourceWillBeModified, false);
  assert(body.blockers.includes("Profile is not approved"));
});

test("cutover apply requires explicit confirmation code", async () => {
  const result = await cli(["cutover", "--profile", profile, "--apply", "--confirm", "WRONG"]);
  assert.notEqual(result.code, 0);
  assert.match(result.stderr, /Cannot apply while plan has blockers/);
});

test("approved cutover still requires its exact confirmation code", async () => {
  const approvedProfile = JSON.parse(await fs.readFile(profile, "utf8"));
  approvedProfile.approved = true;
  const temporaryProfile = path.join(os.tmpdir(), `server-migrate-${process.pid}.yaml`);
  await fs.writeFile(temporaryProfile, JSON.stringify(approvedProfile));
  try {
    const result = await cli(["cutover", "--profile", temporaryProfile, "--apply", "--confirm", "WRONG"]);
    assert.notEqual(result.code, 0);
    assert.match(result.stderr, /Invalid confirmation code/);
  } finally {
    await fs.unlink(temporaryProfile);
  }
});

test("rollback states that source was not changed", async () => {
  const result = await cli(["rollback", "--profile", profile, "--json"]);
  assert.equal(result.code, 0, result.stderr);
  const body = JSON.parse(result.stdout);
  assert.equal(body.sourceChanged, false);
});
