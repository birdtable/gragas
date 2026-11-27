// Written by gragas-generate from internal/host/assets/contract/schema/v1/entity-exclusion.schema.json; edits are lost, so edit the source and run task gen.

import { z } from "zod";
import { Id, Slug, Text, Timestamp } from "./common.gen.js";

export const ExclusionLifted = z.strictObject({
  decision: Slug,
  who: Text,
  when: Timestamp,
});
export type ExclusionLifted = z.infer<typeof ExclusionLifted>;

/** Exclusion of a parent rule by an overlay (entity, spec 16.1). */
export const Exclusion = z.strictObject({
  overlay: Slug,
  rule: Id,
  number: z.number().int().min(1),
  decision: Slug,
  who: Text,
  when: Timestamp,
  lifted: ExclusionLifted.nullable().default(null),
});
export type Exclusion = z.infer<typeof Exclusion>;
