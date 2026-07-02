import { assertServiceName, assertSshAlias } from "./safety.js";
import { run } from "./shell.js";

export class MutableTarget {
  constructor(sshAlias, { runner = run, apply = false } = {}) {
    this.sshAlias = assertSshAlias(sshAlias, "Target SSH alias");
    this.runner = runner;
    this.apply = apply;
  }

  async execute(command, { input } = {}) {
    if (!this.apply) {
      return { dryRun: true, command };
    }
    return this.runner("ssh", [
      "-o", "BatchMode=yes",
      "-o", "ConnectTimeout=10",
      "--",
      this.sshAlias,
      command
    ], { input });
  }

  async service(action, name) {
    if (!["start", "stop", "restart", "enable", "disable"].includes(action)) {
      throw new Error(`Unsupported target service action: ${action}`);
    }
    return this.execute(`sudo systemctl ${action} '${assertServiceName(name)}'`);
  }
}
