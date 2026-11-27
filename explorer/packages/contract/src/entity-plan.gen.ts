// Written by gragas-generate from internal/host/assets/contract/schema/v1/entity-plan.schema.json; edits are lost, so edit the source and run task gen.

import { z } from "zod";
import { Digest, Slug, Standing, Text, Url } from "./common.gen.js";

export const PlanChanges = z.strictObject({
  entity: Text,
  standing_before: Standing.nullable().default(null),
  standing_after: Standing.nullable().default(null),
  what: Text,
});
export type PlanChanges = z.infer<typeof PlanChanges>;

export const PlanWrites = z.strictObject({
  path: Text,
  what: Text,
});
export type PlanWrites = z.infer<typeof PlanWrites>;

export const PlanRekeyedEntity = z.enum(["waiver", "baseline"]);
export type PlanRekeyedEntity = z.infer<typeof PlanRekeyedEntity>;

export const PlanRekeyedState = z.enum(["re-keyed", "orphaned"]);
export type PlanRekeyedState = z.infer<typeof PlanRekeyedState>;

export const PlanRekeyed = z.strictObject({
  entity: PlanRekeyedEntity,
  slug: Slug,
  state: PlanRekeyedState,
});
export type PlanRekeyed = z.infer<typeof PlanRekeyed>;

export const PlanRenderingsWhat = z.enum(["agent file", "review file", "preset copy", "page"]);
export type PlanRenderingsWhat = z.infer<typeof PlanRenderingsWhat>;

export const PlanRenderings = z.strictObject({
  path: Text,
  what: PlanRenderingsWhat,
});
export type PlanRenderings = z.infer<typeof PlanRenderings>;

export const PlanCheckerCode = z.strictObject({
  name: Slug,
  diff: Url.nullable().default(null),
});
export type PlanCheckerCode = z.infer<typeof PlanCheckerCode>;

/** Plan (entity, spec 16.1 and 15.3): one shape for every changing command. */
export const Plan = z.strictObject({
  digest: Digest,
  command: Text,
  changes: z.array(PlanChanges),
  consumer_actions: z.array(Text).default([]),
  writes: z.array(PlanWrites),
  rekeyed: z.array(PlanRekeyed).default([]),
  renderings: z.array(PlanRenderings).default([]),
  checker_code: z.array(PlanCheckerCode).default([]),
  question: Text,
});
export type Plan = z.infer<typeof Plan>;
