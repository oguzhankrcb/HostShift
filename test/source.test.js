import test from "node:test";
import assert from "node:assert/strict";
import { ReadOnlySource } from "../src/source.js";

test("source executor exposes only allowlisted fact reads", async () => {
  const calls = [];
  const source = new ReadOnlySource("old-server", {
    runner: async (command, args) => {
      calls.push({ command, args });
      return { stdout: "ok\n", stderr: "", code: 0 };
    }
  });
  const result = await source.readFact("hostname");
  assert.equal(result.value, "ok");
  assert.equal(calls[0].command, "ssh");
  assert(!calls[0].args.join(" ").includes("sudo"));
  assert.equal(typeof source.execute, "undefined");
});
