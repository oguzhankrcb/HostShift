import path from "node:path";
import { VERSION } from "./constants.js";
import { discoverProfile } from "./discovery.js";
import { doctor } from "./doctor.js";
import { drift } from "./drift.js";
import { cutover, prepare, sync, confirmationCode } from "./operations.js";
import { buildPlan } from "./planner.js";
import { readProfile, writeProfile } from "./profile.js";
import { verify } from "./verify.js";

function parseArgs(argv) {
  const [command = "help", ...rest] = argv;
  const options = { _: [] };
  for (let index = 0; index < rest.length; index += 1) {
    const value = rest[index];
    if (!value.startsWith("--")) {
      options._.push(value);
      continue;
    }
    const key = value.slice(2);
    if (["apply", "json"].includes(key)) {
      options[key] = true;
    } else {
      options[key] = rest[++index];
    }
  }
  return { command, options };
}

function required(options, key) {
  if (!options[key]) throw new Error(`Missing --${key}`);
  return options[key];
}

function output(value, json = false) {
  if (json || typeof value !== "string") {
    process.stdout.write(`${JSON.stringify(value, null, 2)}\n`);
  } else {
    process.stdout.write(`${value}\n`);
  }
}

export async function main(argv) {
  const { command, options } = parseArgs(argv);
  if (command === "help" || options.help) {
    output(helpText());
    return;
  }
  if (command === "version") {
    output(VERSION);
    return;
  }
  if (command === "doctor") {
    output(await doctor({ source: options.source, target: options.target }), options.json);
    return;
  }
  if (command === "discover") {
    const name = required(options, "name");
    const profilePath = options.profile ?? path.resolve(`${name}.profile.yaml`);
    const profile = await discoverProfile({ source: required(options, "source"), name });
    await writeProfile(profilePath, profile);
    output({ profile: profilePath, approved: false, sourcePolicy: profile.source.policy }, options.json);
    return;
  }

  const profile = await readProfile(required(options, "profile"));
  if (options.target) profile.target.ssh = options.target;

  if (command === "plan") {
    output(buildPlan(profile), options.json);
  } else if (command === "prepare") {
    assertCanApply(profile, options.apply);
    output(await prepare(profile, { apply: options.apply }), options.json);
  } else if (command === "sync") {
    assertCanApply(profile, options.apply);
    output(await sync(profile, { apply: options.apply }), options.json);
  } else if (command === "verify") {
    output(await verify(profile), options.json);
  } else if (command === "drift") {
    output(await drift(profile), options.json);
  } else if (command === "cutover") {
    assertCanApply(profile, options.apply);
    if (!options.apply) {
      output({ dryRun: true, confirmationCode: confirmationCode(profile), actions: await cutover(profile) }, options.json);
    } else {
      output(await cutover(profile, { apply: true, confirmation: options.confirm }), options.json);
    }
  } else if (command === "rollback") {
    output({
      automatic: false,
      sourceChanged: false,
      message: "The source was never changed. Keep DNS on the source and inspect the target before stopping target services."
    }, options.json);
  } else {
    throw new Error(`Unknown command: ${command}`);
  }
}

function assertCanApply(profile, apply) {
  if (!apply) return;
  const plan = buildPlan(profile);
  if (plan.blockers.length > 0) {
    throw new Error(`Cannot apply while plan has blockers: ${plan.blockers.join("; ")}`);
  }
}

function helpText() {
  return `server-migrate ${VERSION}

Read-only-source Ubuntu and Debian migration CLI.

Commands:
  doctor   --source <ssh> --target <ssh>
  discover --source <ssh> --name <name> [--profile <file>]
  plan     --profile <file> [--target <ssh>]
  prepare  --profile <file> [--apply]
  sync     --profile <file> [--apply]
  verify   --profile <file>
  cutover  --profile <file> [--apply --confirm <code>]
  rollback --profile <file>
  drift    --profile <file>

Safety:
  The source executor has no arbitrary command method, never uses sudo, and only
  runs audited read-only commands. Mutating commands are target-only.`;
}
