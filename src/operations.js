import {
  buildContainerMysqlDumpCommand,
  buildDatabaseReadCommand,
  buildDockerImageSaveCommand,
  buildSourceTarCommand
} from "./safety.js";
import { DEFAULT_TARGET_POLICY } from "./constants.js";
import { shellQuote } from "./shell.js";
import { streamSshToSsh } from "./stream.js";
import { MutableTarget } from "./target.js";
import { SafetyError } from "./errors.js";

export async function prepare(profile, { apply = false } = {}) {
  const target = new MutableTarget(profile.target.ssh, { apply });
  const packages = [...new Set(profile.packages ?? [])];
  const targetPolicy = profile.targetPolicy ?? DEFAULT_TARGET_POLICY;
  const allowedVersions = targetPolicy.allowedUbuntuVersions ?? DEFAULT_TARGET_POLICY.allowedUbuntuVersions;
  const requiredArchitecture = targetPolicy.requiredArchitecture ?? DEFAULT_TARGET_POLICY.requiredArchitecture;
  const versionCheck = allowedVersions
    .map((version) => `[ \"$VERSION_ID\" = ${shellQuote(version)} ]`)
    .join(" || ");
  const commands = [
    `. /etc/os-release && (${versionCheck})`,
    `test "$(uname -m)" = ${shellQuote(requiredArchitecture)}`
  ];
  if (packages.length > 0) {
    commands.push(`sudo apt-get update && sudo DEBIAN_FRONTEND=noninteractive apt-get install -y ${packages.map(shellQuote).join(" ")}`);
  }
  for (const rule of profile.firewall?.rules ?? []) {
    if (profile.firewall?.enabled !== false) {
      commands.push(`sudo ufw allow from ${shellQuote(rule.from)} to any port ${shellQuote(rule.port)} proto ${shellQuote(rule.proto)}`);
    }
  }
  const sshdSettings = profile.sshd?.settings ?? {};
  if (Object.keys(sshdSettings).length > 0) {
    const content = Object.entries(sshdSettings)
      .map(([key, value]) => `${key} ${value}`)
      .join("\n");
    commands.push(`printf %s ${shellQuote(`${content}\n`)} | sudo tee /etc/ssh/sshd_config.d/99-server-migrate-timeout.conf >/dev/null && sudo sshd -t && sudo systemctl reload ssh`);
  }
  const mysqlSettings = profile.mysql?.settings ?? {};
  if (mysqlSettings.bindAddress || mysqlSettings.mysqlxBindAddress) {
    commands.push(
      `sudo test -f /etc/mysql/mysql.conf.d/mysqld.cnf && (sudo test -f /etc/mysql/mysql.conf.d/mysqld.cnf.server-migrate.bak || sudo cp /etc/mysql/mysql.conf.d/mysqld.cnf /etc/mysql/mysql.conf.d/mysqld.cnf.server-migrate.bak)`
    );
    if (mysqlSettings.bindAddress) {
      commands.push(`sudo sed -i 's/^bind-address\\s*=.*/bind-address\\t\\t= ${mysqlSettings.bindAddress}/' /etc/mysql/mysql.conf.d/mysqld.cnf`);
    }
    if (mysqlSettings.mysqlxBindAddress) {
      commands.push(`sudo sed -i 's/^mysqlx-bind-address\\s*=.*/mysqlx-bind-address\\t= ${mysqlSettings.mysqlxBindAddress}/' /etc/mysql/mysql.conf.d/mysqld.cnf`);
    }
    commands.push("sudo systemctl reload mysql || sudo systemctl restart mysql");
  }
  if (profile.firewall?.enable === true) {
    commands.push("sudo ufw --force enable");
  }
  const results = [];
  for (const command of commands) results.push(await target.execute(command));
  return results;
}

export async function sync(profile, { apply = false } = {}) {
  const actions = [];
  for (const fileSet of profile.fileSets ?? []) {
    const sourceCommand = buildSourceTarCommand(fileSet.paths);
    const targetPath = fileSet.targetPath ?? "/";
    const targetCommand = `sudo tar --extract --file=- --preserve-permissions --same-owner -C ${shellQuote(targetPath)}`;
    actions.push({ sourceCommand, targetCommand, description: `file set ${fileSet.name}` });
  }
  for (const database of profile.databases ?? []) {
    const sourceCommand = buildDatabaseReadCommand(database);
    let targetCommand;
    if (database.engine === "mysql") {
      targetCommand = "mysql";
    } else if (database.engine === "postgresql") {
      targetCommand = `pg_restore --clean --if-exists --no-owner --dbname=${shellQuote(database.targetName ?? database.name)}`;
    } else {
      throw new SafetyError(`No read-only sync strategy for ${database.engine}`);
    }
    actions.push({ sourceCommand, targetCommand, description: `${database.engine} ${database.name}` });
  }
  for (const database of profile.containerDatabases ?? []) {
    const sourceCommand = buildContainerMysqlDumpCommand(database);
    const targetUserEnv = database.targetUserEnv ?? database.userEnv ?? "MYSQL_ROOT_USER";
    const targetPasswordEnv = database.targetPasswordEnv ?? database.passwordEnv ?? "MYSQL_ROOT_PASSWORD";
    const targetScript = `user="\${${targetUserEnv}:-root}"; exec mysql -u"$user" -p"\${${targetPasswordEnv}}"`;
    actions.push({
      sourceCommand,
      targetCommand: `docker exec -i ${shellQuote(database.targetContainer)} sh -c ${shellQuote(targetScript)}`,
      prepareTargetCommand: database.targetCompose
        ? `cd ${shellQuote(database.targetCompose.workingDir)} && docker compose -f ${shellQuote(database.targetCompose.configFile)} up -d ${shellQuote(database.targetCompose.service)}`
        : undefined,
      description: `container mysql ${database.sourceContainer}`
    });
  }
  for (const container of profile.standaloneContainers ?? []) {
    actions.push({
      sourceCommand: buildDockerImageSaveCommand(container.image),
      targetCommand: "docker image load",
      description: `standalone image ${container.image}`
    });
  }
  if (!apply) return actions.map((action) => ({ ...action, dryRun: true }));
  const target = new MutableTarget(profile.target.ssh, { apply: true });
  for (const action of actions) {
    if (action.prepareTargetCommand) {
      await target.execute(action.prepareTargetCommand);
    }
    await streamSshToSsh({
      source: profile.source.ssh,
      sourceCommand: action.sourceCommand,
      target: profile.target.ssh,
      targetCommand: action.targetCommand
    });
  }
  return actions.map((action) => ({ description: action.description, completed: true }));
}

export async function cutover(profile, { apply = false, confirmation } = {}) {
  if (apply && confirmation !== confirmationCode(profile)) {
    throw new SafetyError(`Invalid confirmation code; expected ${confirmationCode(profile)}`);
  }
  const target = new MutableTarget(profile.target.ssh, { apply });
  const results = [];
  for (const project of profile.composeProjects ?? []) {
    results.push(await target.execute(
      `cd ${shellQuote(project.workingDir)} && docker compose -f ${shellQuote(project.configFile)} up -d --build`
    ));
  }
  for (const container of profile.standaloneContainers ?? []) {
    results.push(await target.execute(buildStandaloneRunCommand(container)));
  }
  for (const service of profile.services ?? []) {
    if (service.enabled !== false) results.push(await target.service("enable", service.name));
    results.push(await target.service("start", service.name));
  }
  return results;
}

function buildStandaloneRunCommand(container) {
  const args = [
    "docker", "run", "-d",
    "--name", shellQuote(container.name),
    "--restart", shellQuote(container.restartPolicy ?? "no")
  ];
  for (const [containerPort, bindings] of Object.entries(container.portBindings ?? {})) {
    for (const binding of bindings ?? []) {
      const hostIp = binding.HostIp && !["0.0.0.0", "::"].includes(binding.HostIp) ? `${binding.HostIp}:` : "";
      args.push("-p", shellQuote(`${hostIp}${binding.HostPort}:${containerPort}`));
    }
  }
  for (const [key, value] of Object.entries(container.safeEnvironment ?? {})) {
    args.push("-e", shellQuote(`${key}=${value}`));
  }
  if (container.user) args.push("--user", shellQuote(container.user));
  if (container.workingDir) args.push("--workdir", shellQuote(container.workingDir));
  args.push(shellQuote(container.image));
  return `if docker inspect ${shellQuote(container.name)} >/dev/null 2>&1; then docker start ${shellQuote(container.name)}; else ${args.join(" ")}; fi`;
}

export function confirmationCode(profile) {
  return `START-${profile.name.toUpperCase()}`;
}
