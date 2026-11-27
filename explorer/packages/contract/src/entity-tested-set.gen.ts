// Written by gragas-generate from internal/host/assets/contract/schema/v1/entity-tested-set.schema.json; edits are lost, so edit the source and run task gen.

import { z } from "zod";
import { Semver, Text } from "./common.gen.js";

export const TestedSetTools = z.strictObject({
  tool: Text,
  version: Text,
});
export type TestedSetTools = z.infer<typeof TestedSetTools>;

export const TestedSetPlugins = z.strictObject({
  plugin: Text,
  version: Text,
});
export type TestedSetPlugins = z.infer<typeof TestedSetPlugins>;

export const TestedSetKits = z.strictObject({
  kit: Text,
  version: Text,
});
export type TestedSetKits = z.infer<typeof TestedSetKits>;

export const TestedSetAdapters = z.strictObject({
  adapter: Text,
  version: Text,
});
export type TestedSetAdapters = z.infer<typeof TestedSetAdapters>;

/** Tested set of a release (entity, spec 16.1). */
export const TestedSet = z.strictObject({
  tools: z.array(TestedSetTools),
  plugins: z.array(TestedSetPlugins).default([]),
  kits: z.array(TestedSetKits).default([]),
  adapters: z.array(TestedSetAdapters).default([]),
  minimum_engine: Semver,
});
export type TestedSet = z.infer<typeof TestedSet>;
