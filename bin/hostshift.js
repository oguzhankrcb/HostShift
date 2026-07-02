#!/usr/bin/env node
import fs from "node:fs";
import { spawn } from "node:child_process";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const builtBinary = path.join(root, "dist", process.platform === "win32" ? "hostshift.exe" : "hostshift");
const command = fs.existsSync(builtBinary) ? builtBinary : "go";
const args = fs.existsSync(builtBinary) ? process.argv.slice(2) : ["run", "./cmd/hostshift", ...process.argv.slice(2)];

const child = spawn(command, args, { cwd: root, stdio: "inherit" });
child.on("exit", (code, signal) => {
  if (signal) process.kill(process.pid, signal);
  process.exit(code ?? 1);
});
