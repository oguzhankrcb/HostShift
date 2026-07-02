import { ReadOnlySource } from "./source.js";

export async function drift(profile) {
  const target = new ReadOnlySource(profile.target.ssh);
  const [packages, enabledServices] = await Promise.all([
    target.readFact("packages"),
    target.readFact("enabledServices")
  ]);
  const packageText = packages.value ?? "";
  const serviceText = enabledServices.value ?? "";
  return {
    packages: (profile.packages ?? []).map((name) => ({
      name,
      present: packageText.split("\n").some((line) => line.split("\t")[0] === name)
    })),
    services: (profile.services ?? []).map(({ name }) => ({
      name,
      enabled: serviceText.split("\n").some((line) => line.startsWith(`${name}.service`) || line.startsWith(`${name} `))
    }))
  };
}
