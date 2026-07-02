import path from "node:path";
import { MACHINE_SPECIFIC_EXCLUDES, SOURCE_FORBIDDEN_TOKENS, SOURCE_FACTS } from "./constants.js";
import { SafetyError, ValidationError } from "./errors.js";
import { shellQuote } from "./shell.js";

const SAFE_SSH_ALIAS = /^[a-zA-Z0-9_.@:-]+$/;
const SAFE_SERVICE = /^[a-zA-Z0-9_.@-]+$/;
const SAFE_DOCKER_NAME = /^[a-zA-Z0-9][a-zA-Z0-9_.-]*$/;
const SAFE_DOCKER_IMAGE = /^[a-zA-Z0-9][a-zA-Z0-9_./:@-]*$/;

export function assertSshAlias(value, label = "SSH alias") {
  if (!value || !SAFE_SSH_ALIAS.test(value)) {
    throw new ValidationError(`${label} contains unsafe characters`);
  }
  return value;
}

export function assertServiceName(value) {
  if (!SAFE_SERVICE.test(value)) {
    throw new ValidationError(`Unsafe service name: ${value}`);
  }
  return value;
}

export function assertDockerName(value, label = "Docker name") {
  if (!value || !SAFE_DOCKER_NAME.test(value)) {
    throw new ValidationError(`${label} contains unsafe characters: ${value}`);
  }
  return value;
}

export function assertDockerImage(value) {
  if (!value || !SAFE_DOCKER_IMAGE.test(value)) {
    throw new ValidationError(`Docker image contains unsafe characters: ${value}`);
  }
  return value;
}

export function assertAbsoluteTransferPath(value) {
  if (typeof value !== "string" || !path.posix.isAbsolute(value) || value.includes("\n") || value.includes("\0")) {
    throw new ValidationError(`Transfer path must be a safe absolute path: ${value}`);
  }
  const normalized = path.posix.normalize(value);
  if (normalized === "/" || normalized === "/etc" || normalized === "/var") {
    throw new SafetyError(`Transfer path is too broad: ${normalized}`);
  }
  if (MACHINE_SPECIFIC_EXCLUDES.some((entry) => {
    const base = entry.replace(/\/\*$/, "");
    return normalized === base || normalized.startsWith(`${base}/`);
  })) {
    throw new SafetyError(`Transfer path is machine-specific or unsafe: ${normalized}`);
  }
  return normalized;
}

export function sourceFactCommand(factName) {
  const argv = SOURCE_FACTS[factName];
  if (!argv) {
    throw new SafetyError(`Source command is not allowlisted: ${factName}`);
  }
  return argv.map(shellQuote).join(" ");
}

export function assertReadOnlySourceCommand(command) {
  const lowered = command.toLowerCase();
  for (const token of SOURCE_FORBIDDEN_TOKENS) {
    if (lowered.includes(token.toLowerCase())) {
      throw new SafetyError(`Source command contains forbidden token: ${token}`);
    }
  }
  return command;
}

export function buildSourceTarCommand(paths) {
  const safePaths = paths.map(assertAbsoluteTransferPath);
  const args = safePaths.map((item) => shellQuote(item.replace(/^\//, ""))).join(" ");
  return assertReadOnlySourceCommand(`tar --create --file=- --one-file-system --warning=no-file-changed -C / ${args}`);
}

export function buildDatabaseReadCommand(database) {
  const name = shellQuote(database.name);
  if (database.engine === "mysql") {
    return assertReadOnlySourceCommand(
      `mysqldump --single-transaction --quick --skip-lock-tables --routines --events --triggers --databases ${name}`
    );
  }
  if (database.engine === "postgresql") {
    return assertReadOnlySourceCommand(`pg_dump --format=custom --no-owner --no-acl --dbname=${name}`);
  }
  if (database.engine === "redis") {
    throw new SafetyError("Redis cannot be consistently exported read-only without an existing snapshot or replica");
  }
  throw new ValidationError(`Unsupported database engine: ${database.engine}`);
}

export function buildDockerInspectCommand(containerName) {
  return `docker inspect --type container ${shellQuote(assertDockerName(containerName, "Container name"))}`;
}

export function buildDockerImageSaveCommand(image) {
  return `docker image save ${shellQuote(assertDockerImage(image))}`;
}

export function buildContainerMysqlDumpCommand(database) {
  const container = shellQuote(assertDockerName(database.sourceContainer, "Source DB container"));
  const userEnv = database.userEnv ?? "MYSQL_ROOT_USER";
  const passwordEnv = database.passwordEnv ?? "MYSQL_ROOT_PASSWORD";
  const databaseEnv = database.databaseEnv ?? "MYSQL_DATABASE";
  for (const value of [userEnv, passwordEnv, databaseEnv]) {
    if (!/^[A-Z][A-Z0-9_]*$/.test(value)) {
      throw new ValidationError(`Unsafe container environment variable: ${value}`);
    }
  }
  const script = `user="\${${userEnv}:-root}"; exec mysqldump -u"$user" -p"\${${passwordEnv}}" --single-transaction --quick --routines --events --triggers --databases "\${${databaseEnv}}"`;
  return `docker exec ${container} sh -c ${shellQuote(script)}`;
}
