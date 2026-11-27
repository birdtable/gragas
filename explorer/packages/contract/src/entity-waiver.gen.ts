// Written by gragas-generate from internal/host/assets/contract/schema/v1/entity-waiver.schema.json; edits are lost, so edit the source and run task gen.

import { z } from "zod";
import { Date, Id, Slug } from "./common.gen.js";

export const WaiverKind = z.enum(["path", "kind", "rule"]);
export type WaiverKind = z.infer<typeof WaiverKind>;

/** Waiver as a report counts it (entity, spec 16.6): never the path. */
export const Waiver = z.strictObject({
  slug: Slug,
  rule: Id,
  kind: WaiverKind,
  age_days: z.number().int().min(0),
  expires: Date,
  owner_present: z.boolean(),
});
export type Waiver = z.infer<typeof Waiver>;
