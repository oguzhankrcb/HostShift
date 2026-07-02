import { ReadOnlySource } from "./source.js";
import { createProfile } from "./profile.js";

export async function discoverProfile({ source, name, runner }) {
  const client = new ReadOnlySource(source, { runner });
  const facts = await client.discover();
  return createProfile({ name, source, facts });
}
