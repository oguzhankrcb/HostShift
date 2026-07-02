import { run } from "./shell.js";
import { assertDockerName, assertServiceName, assertSshAlias } from "./safety.js";

export async function verify(profile) {
  const target = assertSshAlias(profile.target.ssh, "Target SSH alias");
  const results = [];
  for (const service of profile.services ?? []) {
    const name = assertServiceName(service.name);
    try {
      await run("ssh", ["-o", "BatchMode=yes", "--", target, `systemctl is-active --quiet '${name}'`]);
      results.push({ type: "service", name, ok: true });
    } catch (error) {
      results.push({ type: "service", name, ok: false, error: error.message });
    }
  }
  for (const container of [
    ...(profile.composeProjects ?? []).flatMap(() => []),
    ...(profile.standaloneContainers ?? [])
  ]) {
    const name = assertDockerName(container.name, "Container");
    try {
      await run("ssh", ["-o", "BatchMode=yes", "--", target, `test "$(docker inspect -f '{{.State.Running}}' '${name}')" = true`]);
      results.push({ type: "container", name, ok: true });
    } catch (error) {
      results.push({ type: "container", name, ok: false, error: error.message });
    }
  }
  for (const check of profile.applicationChecks ?? []) {
    const container = assertDockerName(check.container, "Laravel container");
    const command = `docker exec '${container}' sh -lc 'php artisan tinker --execute="DB::connection()->getPdo(); DB::select(\\"select 1 as ok\\");"'`;
    try {
      await run("ssh", ["-o", "BatchMode=yes", "--", target, command]);
      results.push({ type: "laravelDatabase", name: check.name ?? container, ok: true });
    } catch (error) {
      results.push({ type: "laravelDatabase", name: check.name ?? container, ok: false, error: error.message });
    }
  }
  for (const check of profile.healthChecks ?? []) {
    if (check.type !== "http" || !/^https?:\/\//.test(check.url)) {
      results.push({ type: check.type, name: check.name, ok: false, error: "Unsupported health check" });
      continue;
    }
    try {
      const args = ["--fail", "--silent", "--show-error", "--max-time", String(check.timeoutSeconds ?? 10)];
      if (check.hostHeader) args.push("--header", `Host: ${check.hostHeader}`);
      args.push(check.url);
      await run("curl", args);
      results.push({ type: "http", name: check.name, ok: true });
    } catch (error) {
      results.push({ type: "http", name: check.name, ok: false, error: error.message });
    }
  }
  return results;
}
