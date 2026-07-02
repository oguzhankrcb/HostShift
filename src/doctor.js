import { commandExists, run } from "./shell.js";
import { assertSshAlias } from "./safety.js";

export async function doctor({ source, target }) {
  const checks = [];
  for (const command of ["ssh", "node"]) {
    checks.push({ name: `local:${command}`, ok: await commandExists(command) });
  }

  for (const [role, alias] of [["source", source], ["target", target]]) {
    if (!alias) continue;
    assertSshAlias(alias, `${role} SSH alias`);
    try {
      const result = await run("ssh", [
        "-o", "BatchMode=yes",
        "-o", "ConnectTimeout=10",
        "--", alias, "'printf' 'connected'"
      ]);
      checks.push({ name: `${role}:ssh`, ok: result.stdout === "connected" });
      if (role === "target") {
        const release = await run("ssh", [
          "-o", "BatchMode=yes", "--", alias,
          ". /etc/os-release; printf '%s|%s' \"$VERSION_ID\" \"$VERSION_CODENAME\""
        ]);
        const [version, codename] = release.stdout.split("|");
        checks.push({ name: "target:ubuntu-release", ok: true, version, codename });
        const upgrade = await run("ssh", [
          "-o", "BatchMode=yes", "--", alias,
          "do-release-upgrade -c 2>&1 || true"
        ]);
        checks.push({
          name: "target:release-support",
          ok: !upgrade.stdout.includes("not supported anymore"),
          detail: upgrade.stdout.trim()
        });
      }
    } catch (error) {
      checks.push({ name: `${role}:ssh`, ok: false, error: error.message });
    }
  }
  return checks;
}
