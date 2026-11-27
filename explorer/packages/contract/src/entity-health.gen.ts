// Written by gragas-generate from internal/host/assets/contract/schema/v1/entity-health.schema.json; edits are lost, so edit the source and run task gen.

import { z } from "zod";
import { Slug } from "./common.gen.js";

export const HealthNumbers = z.strictObject({
  name: Slug,
  value: z.number(),
});
export type HealthNumbers = z.infer<typeof HealthNumbers>;

/** The seven health numbers of a release (entity, spec 16.1). */
export const Health = z.strictObject({
  numbers: z.array(HealthNumbers).min(7).max(7),
  resolved: z.boolean().default(false),
});
export type Health = z.infer<typeof Health>;
