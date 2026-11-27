// Written by gragas-generate from internal/host/assets/contract/schema/v1/entity-rendering-manifest.schema.json; edits are lost, so edit the source and run task gen.

import { z } from "zod";
import { Digest, Id, Language, Semver, Standing, Text, Timestamp } from "./common.gen.js";

export const RenderingManifestRenderingsRules = z.strictObject({
  rule: Id,
  standing: Standing,
});
export type RenderingManifestRenderingsRules = z.infer<typeof RenderingManifestRenderingsRules>;

export const RenderingManifestRenderingsOmissions = z.strictObject({
  what: Text,
  who: Text,
  when: Timestamp,
  plan: Digest,
});
export type RenderingManifestRenderingsOmissions = z.infer<typeof RenderingManifestRenderingsOmissions>;

export const RenderingManifestRenderings = z.strictObject({
  path: Text,
  source_hash: Digest,
  language: Language,
  rules: z.array(RenderingManifestRenderingsRules).default([]),
  omissions: z.array(RenderingManifestRenderingsOmissions).default([]),
});
export type RenderingManifestRenderings = z.infer<typeof RenderingManifestRenderings>;

/** Rendering manifest of a release and target (entity, spec 16.1). */
export const RenderingManifest = z.strictObject({
  release: Semver,
  target: Text,
  renderings: z.array(RenderingManifestRenderings),
});
export type RenderingManifest = z.infer<typeof RenderingManifest>;
