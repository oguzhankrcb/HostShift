import { buildDatabaseReadCommand, buildSourceTarCommand } from "./safety.js";
import { buildContainerMysqlDumpCommand, buildDockerImageSaveCommand } from "./safety.js";
import { DEFAULT_TARGET_POLICY } from "./constants.js";

export function buildPlan(profile) {
  const blockers = [];
  const warnings = [];
  const steps = [];

  if (!profile.approved) {
    blockers.push("Profile is not approved");
  }
  if (!profile.target?.ssh) {
    blockers.push("Target SSH alias is missing");
  }
  const targetPolicy = profile.targetPolicy ?? DEFAULT_TARGET_POLICY;
  const allowedVersions = targetPolicy.allowedUbuntuVersions ?? DEFAULT_TARGET_POLICY.allowedUbuntuVersions;
  const preferredVersions = targetPolicy.preferredUbuntuVersions ?? DEFAULT_TARGET_POLICY.preferredUbuntuVersions;
  warnings.push(`Target Ubuntu policy allows ${allowedVersions.join(", ")}; preferred ${preferredVersions.join(", ")}`);
  for (const version of allowedVersions) {
    if (!preferredVersions.includes(version)) {
      warnings.push(`Ubuntu ${version} is allowed but not the preferred baseline; package/runtime compatibility must be verified before cutover`);
    }
  }

  steps.push({
    phase: "prepare",
    mutates: "target-only",
    description: `Validate Ubuntu ${allowedVersions.join(" or ")} ${targetPolicy.requiredArchitecture ?? DEFAULT_TARGET_POLICY.requiredArchitecture} target and install approved packages`
  });
  if (profile.firewall?.enabled !== false) {
    steps.push({
      phase: "prepare",
      mutates: "target-only",
      description: `Apply ${profile.firewall?.rules?.length ?? 0} UFW firewall rules, including Docker-to-MySQL access`
    });
  }
  if (profile.sshd?.settings) {
    steps.push({
      phase: "prepare",
      mutates: "target-only",
      description: "Apply sshd keepalive timeout settings and reload sshd on the target"
    });
  }
  if (profile.mysql?.settings) {
    steps.push({
      phase: "prepare",
      mutates: "target-only",
      description: "Apply MySQL bind-address settings for Docker bridge access on the target"
    });
  }

  for (const fileSet of profile.fileSets ?? []) {
    const command = buildSourceTarCommand(fileSet.paths);
    steps.push({
      phase: "sync",
      mutates: "target-only",
      description: `Stream file set ${fileSet.name} from source stdout to target`,
      sourceCommand: command,
      targetPath: fileSet.targetPath ?? "/"
    });
  }

  for (const database of profile.databases ?? []) {
    try {
      const sourceCommand = buildDatabaseReadCommand(database);
      steps.push({
        phase: "sync",
        mutates: "target-only",
        description: `Stream ${database.engine} database ${database.name}`,
        sourceCommand
      });
    } catch (error) {
      blockers.push(error.message);
    }
  }

  const volumePolicies = new Map((profile.volumePolicies ?? []).map((policy) => [policy.volume, policy]));
  for (const risk of profile.containerDataRisks ?? []) {
    const policy = volumePolicies.get(risk.volume);
    if (!policy) {
      blockers.push(`Docker volume ${risk.volume} used by ${risk.container} has no migration policy`);
    } else {
      steps.push({
        phase: "sync",
        mutates: policy.strategy === "discard" ? "none" : "target-only",
        description: `Handle Docker volume ${risk.volume} using ${policy.strategy}`
      });
    }
  }

  for (const database of profile.containerDatabases ?? []) {
    steps.push({
      phase: "sync",
      mutates: "target-only",
      description: `Stream MySQL from container ${database.sourceContainer} to ${database.targetContainer}`,
      sourceCommand: buildContainerMysqlDumpCommand(database)
    });
  }

  for (const container of profile.standaloneContainers ?? []) {
    if ((container.secretEnvironmentKeys ?? []).length > 0) {
      blockers.push(`Standalone container ${container.name} has unresolved environment keys: ${container.secretEnvironmentKeys.join(", ")}`);
    }
    steps.push({
      phase: "sync",
      mutates: "target-only",
      description: `Stream standalone Docker image ${container.image}`,
      sourceCommand: buildDockerImageSaveCommand(container.image)
    });
    steps.push({
      phase: "cutover",
      mutates: "target-only",
      description: `Run standalone container ${container.name} from ${container.image}`
    });
  }

  if ((profile.fileSets ?? []).length === 0) {
    warnings.push("No file sets are configured");
  }
  if ((profile.healthChecks ?? []).length === 0) {
    warnings.push("No health checks are configured");
  }
  if ((profile.applicationChecks ?? []).length === 0 && (profile.databases ?? []).length > 0) {
    warnings.push("No application-level database checks are configured");
  }

  steps.push({
    phase: "verify",
    mutates: "none",
    description: "Run target service, port, HTTP, and checksum checks"
  });
  for (const project of profile.composeProjects ?? []) {
    steps.push({
      phase: "cutover",
      mutates: "target-only",
      description: `Run docker compose up -d for ${project.name}`,
      workingDir: project.workingDir,
      configFile: project.configFile
    });
  }
  steps.push({
    phase: "cutover",
    mutates: "target-only",
    description: "Start target services; DNS remains a manual operation"
  });

  return {
    sourcePolicy: "strict-read-only",
    sourceWillBeModified: false,
    blockers,
    warnings,
    steps
  };
}
