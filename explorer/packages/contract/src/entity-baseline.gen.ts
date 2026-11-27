// Written by gragas-generate from internal/host/assets/contract/schema/v1/entity-baseline.schema.json; edits are lost, so edit the source and run task gen.

import { z } from "zod";
import { Date, Id, Slug, Text } from "./common.gen.js";

/** Baseline as a report counts it (entity, spec 16.7). */
export const Baseline = z.strictObject({
  slug: Slug,
  rule: Id,
  tool: Text,
  count: z.number().int().min(0),
  owner: Text,
  zero_by: Date,
  age_days: z.number().int().min(0).default(0),
});
export type Baseline = z.infer<typeof Baseline>;
