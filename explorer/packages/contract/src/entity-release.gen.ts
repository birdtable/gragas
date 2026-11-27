// Written by gragas-generate from internal/host/assets/contract/schema/v1/entity-release.schema.json; edits are lost, so edit the source and run task gen.

import { z } from "zod";
import { Contract, Date, Digest, Reason, Semver, Slug, Text, TextOrEmpty } from "./common.gen.js";
import { Health } from "./entity-health.gen.js";
import { TestedSet } from "./entity-tested-set.gen.js";

export const ReleaseBump = z.enum(["major", "minor", "patch"]);
export type ReleaseBump = z.infer<typeof ReleaseBump>;

export const ReleaseAssets = z.strictObject({
  path: Text,
  digest: Digest,
});
export type ReleaseAssets = z.infer<typeof ReleaseAssets>;

export const ReleaseCheckerCode = z.strictObject({
  name: Slug,
  publisher: Text,
  digest: Digest,
});
export type ReleaseCheckerCode = z.infer<typeof ReleaseCheckerCode>;

export const ReleaseWithdrawn = z.strictObject({
  date: Date,
  reason: Reason,
});
export type ReleaseWithdrawn = z.infer<typeof ReleaseWithdrawn>;

/** Release (entity, spec 16.1). */
export const Release = z.strictObject({
  version: Semver,
  bump: ReleaseBump,
  date: Date,
  release_note: TextOrEmpty.default(""),
  assets: z.array(ReleaseAssets).default([]),
  checker_code: z.array(ReleaseCheckerCode).default([]),
  tested_set: TestedSet.nullable().default(null),
  health: Health.nullable().default(null),
  format: z.number().int().min(1),
  contract: Contract,
  withdrawn: ReleaseWithdrawn.nullable().default(null),
});
export type Release = z.infer<typeof Release>;
