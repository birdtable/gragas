// Written by gragas-generate from internal/host/assets/contract/schema/v1/entity-preset.schema.json; edits are lost, so edit the source and run task gen.

import { z } from "zod";
import { Digest, Semver, Slug, Text, TextOrEmpty } from "./common.gen.js";

export const PresetExtends = z.strictObject({
  rulebook: Text,
  version: Semver,
});
export type PresetExtends = z.infer<typeof PresetExtends>;

/** Preset by name and digest (entity, spec 16.1). */
export const Preset = z.strictObject({
  tool: Text,
  name: Slug,
  release: Semver,
  digest: Digest,
  entries: z.number().int().min(0).default(0),
  extends: z.array(PresetExtends).default([]),
  licence: TextOrEmpty.default(""),
});
export type Preset = z.infer<typeof Preset>;
