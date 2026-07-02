#!/usr/bin/env node
import fs from "node:fs";
import path from "node:path";
import { spawnSync } from "node:child_process";

const output = process.argv[2] ?? "dist/hostshift.sbom.spdx.json";
const goCache = process.env.GOCACHE || path.join(process.cwd(), ".cache/go-build");

fs.mkdirSync(goCache, { recursive: true });

const result = spawnSync("go", ["list", "-m", "all"], {
  encoding: "utf8",
  env: {
    ...process.env,
    GOCACHE: goCache
  }
});

if (result.status !== 0) {
  process.stderr.write(result.stderr || result.stdout || "go list failed\n");
  process.exit(result.status ?? 1);
}

const modules = result.stdout
  .trim()
  .split(/\r?\n/)
  .filter(Boolean)
  .map((line) => {
    const [name, version = ""] = line.trim().split(/\s+/);
    return { name, version };
  });

const now = new Date().toISOString();
const packages = modules.map((mod, index) => {
  const id = `SPDXRef-Package-${sanitizeID(mod.name)}-${index + 1}`;
  const purl = mod.version ? `pkg:golang/${encodeURIComponent(mod.name)}@${encodeURIComponent(mod.version)}` : `pkg:golang/${encodeURIComponent(mod.name)}`;
  return {
    name: mod.name,
    SPDXID: id,
    versionInfo: mod.version || "main",
    downloadLocation: mod.version ? `https://${mod.name}` : "NOASSERTION",
    filesAnalyzed: false,
    licenseConcluded: "NOASSERTION",
    licenseDeclared: "NOASSERTION",
    copyrightText: "NOASSERTION",
    externalRefs: [
      {
        referenceCategory: "PACKAGE-MANAGER",
        referenceType: "purl",
        referenceLocator: purl
      }
    ]
  };
});

const document = {
  spdxVersion: "SPDX-2.3",
  dataLicense: "CC0-1.0",
  SPDXID: "SPDXRef-DOCUMENT",
  name: "HostShift Go module dependency SBOM",
  documentNamespace: `https://github.com/oguzhankaracabay/hostshift/sbom/${Date.now()}`,
  creationInfo: {
    created: now,
    creators: ["Tool: hostshift-sbom-script"]
  },
  packages,
  relationships: packages.slice(0, 1).map((pkg) => ({
    spdxElementId: "SPDXRef-DOCUMENT",
    relationshipType: "DESCRIBES",
    relatedSpdxElement: pkg.SPDXID
  }))
};

fs.mkdirSync(path.dirname(output), { recursive: true });
fs.writeFileSync(output, `${JSON.stringify(document, null, 2)}\n`);

function sanitizeID(value) {
  return value.replace(/[^A-Za-z0-9.-]/g, "-").replace(/^-+|-+$/g, "") || "module";
}
