import fs from "node:fs/promises";
import path from "node:path";
import {
  DEFAULT_FIREWALL_RULES,
  DEFAULT_MYSQL_SETTINGS,
  DEFAULT_SSHD_SETTINGS,
  DEFAULT_TARGET_POLICY,
  MACHINE_SPECIFIC_EXCLUDES,
  PROFILE_VERSION
} from "./constants.js";
import { ValidationError } from "./errors.js";
import {
  assertAbsoluteTransferPath,
  assertDockerImage,
  assertDockerName,
  assertServiceName,
  assertSshAlias
} from "./safety.js";

export function createProfile({ name, source, facts }) {
  const composeFileSets = composeProjectFileSets(facts);
  const composeProjects = composeProjectsFromFacts(facts);
  const standaloneContainers = standaloneContainersFromFacts(facts);
  const containerDataRisks = containerDataRisksFromFacts(facts);
  const databases = mysqlDatabases(facts);
  const hasLetsEncrypt = Boolean(facts.letsEncryptFiles?.ok && facts.letsEncryptFiles.value.trim());
  return {
    schemaVersion: PROFILE_VERSION,
    name,
    source: {
      ssh: assertSshAlias(source),
      policy: "strict-read-only"
    },
    target: {
      ssh: ""
    },
    targetPolicy: { ...DEFAULT_TARGET_POLICY },
    discoveredAt: new Date().toISOString(),
    sourceFacts: facts,
    packages: [],
    services: [],
    firewall: {
      enabled: true,
      enable: true,
      rules: [...DEFAULT_FIREWALL_RULES]
    },
    sshd: {
      settings: { ...DEFAULT_SSHD_SETTINGS }
    },
    mysql: {
      settings: { ...DEFAULT_MYSQL_SETTINGS }
    },
    composeProjects,
    standaloneContainers,
    containerDatabases: [],
    containerDataRisks,
    volumePolicies: [],
    fileSets: [
      ...composeFileSets,
      { name: "nginx-config", paths: ["/etc/nginx"], targetPath: "/" },
      ...(hasLetsEncrypt ? [{ name: "letsencrypt", paths: ["/etc/letsencrypt"], targetPath: "/" }] : [])
    ],
    databases,
    healthChecks: [],
    applicationChecks: [],
    excludes: [...MACHINE_SPECIFIC_EXCLUDES],
    approved: false
  };
}

function containerDetails(facts) {
  return facts.dockerContainerDetails?.ok && Array.isArray(facts.dockerContainerDetails.value)
    ? facts.dockerContainerDetails.value
    : [];
}

function standaloneContainersFromFacts(facts) {
  return containerDetails(facts)
    .filter((container) => !container.labels?.["com.docker.compose.project"])
    .map((container) => ({
      name: container.name,
      image: container.image,
      restartPolicy: container.restartPolicy,
      portBindings: container.portBindings,
      user: container.user,
      workingDir: container.workingDir,
      safeEnvironment: container.safeEnvironment,
      secretEnvironmentKeys: container.secretEnvironmentKeys
    }));
}

function containerDataRisksFromFacts(facts) {
  return containerDetails(facts).flatMap((container) =>
    container.mounts
      .filter((mount) => mount.type === "volume")
      .map((mount) => ({
        volume: mount.name,
        container: container.name,
        image: container.image,
        destination: mount.destination,
        project: container.labels?.["com.docker.compose.project"] ?? "",
        service: container.labels?.["com.docker.compose.service"] ?? ""
      }))
  );
}

function composeProjectFileSets(facts) {
  return composeProjectsFromFacts(facts)
    .map((project) => project.workingDir)
    .filter((dir, index, dirs) => dirs.indexOf(dir) === index)
    .map((dir) => ({
      name: dir.replace(/^\/+/, "").replaceAll("/", "-"),
      paths: [dir],
      targetPath: "/"
    }));
}

function composeProjectsFromFacts(facts) {
  if (!facts.dockerComposeProjects?.ok || !facts.dockerComposeProjects.value) return [];
  try {
    const projects = JSON.parse(facts.dockerComposeProjects.value);
    return projects
      .map((project) => {
        const configFile = project.ConfigFiles?.split(",")?.[0];
        if (!configFile) return null;
        return {
          name: project.Name,
          configFile,
          workingDir: configFile.replace(/\/[^/]+$/, "")
        };
      })
      .filter(Boolean);
  } catch {
    return [];
  }
}

function mysqlDatabases(facts) {
  if (!facts.mysqlDatabases?.ok || !facts.mysqlDatabases.value) return [];
  const systemDatabases = new Set(["information_schema", "mysql", "performance_schema", "sys"]);
  return facts.mysqlDatabases.value
    .split("\n")
    .map((line) => line.trim())
    .filter((line) => line && !systemDatabases.has(line))
    .map((name) => ({ engine: "mysql", name }));
}

export async function readProfile(filePath) {
  const raw = await fs.readFile(filePath, "utf8");
  let profile;
  try {
    profile = JSON.parse(raw);
  } catch {
    throw new ValidationError("profile.yaml must use JSON-compatible YAML syntax");
  }
  validateProfile(profile);
  return profile;
}

export async function writeProfile(filePath, profile) {
  await fs.mkdir(path.dirname(path.resolve(filePath)), { recursive: true });
  await fs.writeFile(filePath, `${JSON.stringify(profile, null, 2)}\n`, { mode: 0o600 });
}

export function validateProfile(profile) {
  if (!profile || profile.schemaVersion !== PROFILE_VERSION) {
    throw new ValidationError(`Unsupported profile schema; expected ${PROFILE_VERSION}`);
  }
  if (!profile.name || !/^[a-zA-Z0-9_-]+$/.test(profile.name)) {
    throw new ValidationError("Profile name must contain only letters, digits, underscores, and hyphens");
  }
  assertSshAlias(profile.source?.ssh, "Source SSH alias");
  if (profile.source?.policy !== "strict-read-only") {
    throw new ValidationError("Source policy must be strict-read-only");
  }
  if (profile.target?.ssh) {
    assertSshAlias(profile.target.ssh, "Target SSH alias");
  }
  validateTargetPolicy(profile.targetPolicy);
  for (const packageName of profile.packages ?? []) {
    if (!/^[a-zA-Z0-9.+:-]+$/.test(packageName)) {
      throw new ValidationError(`Unsafe package name: ${packageName}`);
    }
  }
  for (const service of profile.services ?? []) {
    assertServiceName(service.name);
  }
  for (const project of profile.composeProjects ?? []) {
    if (!project.name || !/^[a-zA-Z0-9_.-]+$/.test(project.name)) {
      throw new ValidationError(`Invalid compose project name: ${project.name}`);
    }
    assertAbsoluteTransferPath(project.workingDir);
    assertAbsoluteTransferPath(project.configFile);
  }
  for (const container of profile.standaloneContainers ?? []) {
    assertDockerName(container.name, "Standalone container name");
    assertDockerImage(container.image);
    if (!["no", "always", "unless-stopped", "on-failure"].includes(container.restartPolicy ?? "no")) {
      throw new ValidationError(`Invalid restart policy: ${container.restartPolicy}`);
    }
  }
  for (const database of profile.containerDatabases ?? []) {
    if (database.engine !== "mysql") {
      throw new ValidationError(`Unsupported container database engine: ${database.engine}`);
    }
    assertDockerName(database.sourceContainer, "Source DB container");
    assertDockerName(database.targetContainer, "Target DB container");
    if (database.targetCompose) {
      assertAbsoluteTransferPath(database.targetCompose.workingDir);
      assertAbsoluteTransferPath(database.targetCompose.configFile);
      assertDockerName(database.targetCompose.service, "Target compose service");
    }
  }
  for (const policy of profile.volumePolicies ?? []) {
    assertDockerName(policy.volume, "Docker volume");
    if (!["discard", "container-database"].includes(policy.strategy)) {
      throw new ValidationError(`Invalid volume strategy: ${policy.strategy}`);
    }
  }
  validateFirewall(profile.firewall);
  validateSshd(profile.sshd);
  validateMysql(profile.mysql);
  for (const fileSet of profile.fileSets ?? []) {
    if (!Array.isArray(fileSet.paths) || fileSet.paths.length === 0) {
      throw new ValidationError(`File set ${fileSet.name ?? "<unnamed>"} has no paths`);
    }
    fileSet.paths.forEach(assertAbsoluteTransferPath);
  }
  for (const database of profile.databases ?? []) {
    if (!["mysql", "postgresql", "redis"].includes(database.engine)) {
      throw new ValidationError(`Unsupported database engine: ${database.engine}`);
    }
    if (!database.name || /[\n\0]/.test(database.name)) {
      throw new ValidationError("Database name is required and cannot contain control characters");
    }
  }
  for (const check of profile.applicationChecks ?? []) {
    if (check.type !== "laravelDatabase") {
      throw new ValidationError(`Unsupported application check: ${check.type}`);
    }
    assertDockerName(check.container, "Laravel container");
  }
  return profile;
}

function validateFirewall(firewall = {}) {
  if (!firewall.rules) return;
  for (const rule of firewall.rules) {
    if (!/^[0-9a-fA-F:./]+$/.test(rule.from)) {
      throw new ValidationError(`Invalid firewall source: ${rule.from}`);
    }
    if (!Number.isInteger(rule.port) || rule.port < 1 || rule.port > 65535) {
      throw new ValidationError(`Invalid firewall port: ${rule.port}`);
    }
    if (!["tcp", "udp"].includes(rule.proto)) {
      throw new ValidationError(`Invalid firewall protocol: ${rule.proto}`);
    }
  }
  if (firewall.enable !== undefined && typeof firewall.enable !== "boolean") {
    throw new ValidationError("firewall.enable must be a boolean");
  }
}

function validateSshd(sshd = {}) {
  const settings = sshd.settings ?? {};
  for (const [key, value] of Object.entries(settings)) {
    if (!["ClientAliveInterval", "ClientAliveCountMax"].includes(key)) {
      throw new ValidationError(`Unsupported sshd setting: ${key}`);
    }
    if (!Number.isInteger(value) || value < 0 || value > 86400) {
      throw new ValidationError(`Invalid sshd setting ${key}: ${value}`);
    }
  }
}

function validateMysql(mysql = {}) {
  const settings = mysql.settings ?? {};
  for (const key of ["bindAddress", "mysqlxBindAddress"]) {
    if (settings[key] && !/^[0-9a-fA-F:.]+$/.test(settings[key])) {
      throw new ValidationError(`Invalid MySQL ${key}: ${settings[key]}`);
    }
  }
}

function validateTargetPolicy(policy = DEFAULT_TARGET_POLICY) {
  const versions = policy.allowedUbuntuVersions ?? DEFAULT_TARGET_POLICY.allowedUbuntuVersions;
  if (!Array.isArray(versions) || versions.length === 0) {
    throw new ValidationError("targetPolicy.allowedUbuntuVersions must be a non-empty array");
  }
  for (const version of versions) {
    if (!/^\d{2}\.\d{2}$/.test(version)) {
      throw new ValidationError(`Invalid Ubuntu version in target policy: ${version}`);
    }
  }
  const architecture = policy.requiredArchitecture ?? DEFAULT_TARGET_POLICY.requiredArchitecture;
  if (!/^[a-zA-Z0-9_]+$/.test(architecture)) {
    throw new ValidationError(`Invalid target architecture: ${architecture}`);
  }
}
