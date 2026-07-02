import { SOURCE_FACTS } from "./constants.js";
import { buildDockerInspectCommand, sourceFactCommand, assertSshAlias } from "./safety.js";
import { run } from "./shell.js";

export class ReadOnlySource {
  constructor(sshAlias, { runner = run } = {}) {
    this.sshAlias = assertSshAlias(sshAlias, "Source SSH alias");
    this.runner = runner;
  }

  async readFact(name, { optional = false } = {}) {
    const command = sourceFactCommand(name);
    try {
      const result = await this.runner("ssh", [
        "-o", "BatchMode=yes",
        "-o", "ConnectTimeout=10",
        "--",
        this.sshAlias,
        command
      ]);
      return { ok: true, value: result.stdout.trim() };
    } catch (error) {
      if (optional) {
        return { ok: false, error: error.message };
      }
      throw error;
    }
  }

  async discover() {
    const facts = {};
    for (const name of Object.keys(SOURCE_FACTS)) {
      facts[name] = await this.readFact(name, {
        optional: [
          "cron",
          "dockerVersion",
          "dockerComposeProjects",
          "dockerContainers",
          "dockerNetworks",
          "listeners",
          "ufwStatus",
          "sshdEffectiveConfig",
          "sshdConfig",
          "mysqlServerConfig",
          "mysqlDatabases",
          "nginxConfigDump",
          "letsEncryptFiles"
        ].includes(name)
      });
    }
    facts.dockerContainerDetails = await this.readContainerDetails(facts.dockerContainers);
    return facts;
  }

  async readContainerDetails(containerFact) {
    if (!containerFact?.ok || !containerFact.value) return { ok: true, value: [] };
    const summaries = containerFact.value.split("\n").filter(Boolean).map((line) => JSON.parse(line));
    const details = [];
    for (const summary of summaries) {
      const result = await this.runner("ssh", [
        "-o", "BatchMode=yes",
        "-o", "ConnectTimeout=10",
        "--",
        this.sshAlias,
        buildDockerInspectCommand(summary.Names)
      ]);
      const [raw] = JSON.parse(result.stdout);
      details.push(sanitizeContainer(raw));
    }
    return { ok: true, value: details };
  }
}

function sanitizeContainer(raw) {
  const environment = Object.fromEntries((raw.Config?.Env ?? []).map((entry) => {
    const index = entry.indexOf("=");
    return index === -1 ? [entry, ""] : [entry.slice(0, index), entry.slice(index + 1)];
  }));
  const safeEnvNames = new Set(["NODE_ENV", "PORT", "HOSTNAME"]);
  return {
    name: raw.Name?.replace(/^\//, ""),
    image: raw.Config?.Image,
    labels: raw.Config?.Labels ?? {},
    restartPolicy: raw.HostConfig?.RestartPolicy?.Name || "no",
    portBindings: raw.HostConfig?.PortBindings ?? {},
    user: raw.Config?.User ?? "",
    workingDir: raw.Config?.WorkingDir ?? "",
    entrypoint: raw.Config?.Entrypoint ?? [],
    cmd: raw.Config?.Cmd ?? [],
    safeEnvironment: Object.fromEntries(Object.entries(environment).filter(([key]) => safeEnvNames.has(key))),
    secretEnvironmentKeys: Object.keys(environment).filter((key) => !safeEnvNames.has(key) && !["PATH", "NODE_VERSION", "YARN_VERSION"].includes(key)),
    mounts: (raw.Mounts ?? []).map(({ Type, Name, Source, Destination, RW }) => ({
      type: Type,
      name: Name ?? "",
      source: Type === "bind" ? Source : "",
      destination: Destination,
      readWrite: RW
    }))
  };
}
