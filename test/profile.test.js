import test from "node:test";
import assert from "node:assert/strict";
import { validateProfile } from "../src/profile.js";
import { buildPlan } from "../src/planner.js";

function profile(overrides = {}) {
  return {
    schemaVersion: 1,
    name: "app",
    source: { ssh: "old-server", policy: "strict-read-only" },
    target: { ssh: "new-server" },
    packages: [],
    services: [],
    composeProjects: [
      { name: "app", workingDir: "/srv/app", configFile: "/srv/app/docker-compose.yml" }
    ],
    firewall: {
      enabled: true,
      rules: [
        { from: "172.18.0.0/16", port: 3306, proto: "tcp" }
      ]
    },
    sshd: {
      settings: {
        ClientAliveInterval: 120,
        ClientAliveCountMax: 720
      }
    },
    mysql: {
      settings: {
        bindAddress: "172.17.0.1",
        mysqlxBindAddress: "127.0.0.1"
      }
    },
    fileSets: [],
    databases: [],
    healthChecks: [],
    approved: true,
    ...overrides
  };
}

test("profile requires strict source policy", () => {
  assert.throws(
    () => validateProfile(profile({ source: { ssh: "old-server", policy: "mutable" } })),
    /strict-read-only/
  );
});

test("plan explicitly records that source is not modified", () => {
  const result = buildPlan(validateProfile(profile()));
  assert.equal(result.sourceWillBeModified, false);
  assert.equal(result.sourcePolicy, "strict-read-only");
  assert.equal(result.blockers.length, 0);
});

test("plan allows non-preferred Ubuntu targets with a warning", () => {
  const result = buildPlan(validateProfile(profile({
    targetPolicy: {
      allowedUbuntuVersions: ["24.04", "24.10"],
      preferredUbuntuVersions: ["24.04"],
      requiredArchitecture: "x86_64"
    }
  })));
  assert.equal(result.blockers.length, 0);
  assert(result.warnings.some((warning) => warning.includes("24.10")));
});

test("unapproved profile blocks execution plan", () => {
  const result = buildPlan(validateProfile(profile({ approved: false })));
  assert(result.blockers.includes("Profile is not approved"));
});

test("profile validates first-install firewall ssh and mysql settings", () => {
  const result = buildPlan(validateProfile(profile()));
  assert(result.steps.some((step) => step.description.includes("UFW firewall")));
  assert(result.steps.some((step) => step.description.includes("sshd keepalive")));
  assert(result.steps.some((step) => step.description.includes("MySQL bind-address")));
  assert(result.steps.some((step) => step.description.includes("docker compose up")));
});

test("unresolved Docker named volume blocks migration", () => {
  const result = buildPlan(validateProfile(profile({
    containerDataRisks: [
      { volume: "app_mysql", container: "app-db", image: "mysql:8", destination: "/var/lib/mysql" }
    ],
    volumePolicies: []
  })));
  assert(result.blockers.some((blocker) => blocker.includes("app_mysql")));
});

test("standalone container is planned as image stream and cutover", () => {
  const result = buildPlan(validateProfile(profile({
    standaloneContainers: [{
      name: "portfolio",
      image: "portfolio:latest",
      restartPolicy: "always",
      portBindings: { "3000/tcp": [{ HostIp: "", HostPort: "3000" }] },
      safeEnvironment: { NODE_ENV: "production" },
      secretEnvironmentKeys: []
    }]
  })));
  assert(result.steps.some((step) => step.description.includes("standalone Docker image")));
  assert(result.steps.some((step) => step.description.includes("standalone container")));
});

test("standalone secret environment keys block migration", () => {
  const result = buildPlan(validateProfile(profile({
    standaloneContainers: [{
      name: "portfolio",
      image: "portfolio:latest",
      restartPolicy: "always",
      secretEnvironmentKeys: ["DATABASE_URL"]
    }]
  })));
  assert(result.blockers.some((blocker) => blocker.includes("DATABASE_URL")));
});
