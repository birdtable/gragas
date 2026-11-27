// Written by gragas-generate from internal/host/assets/contract/schema/v1/entity-decision.schema.json; edits are lost, so edit the source and run task gen.

import { z } from "zod";
import { Date, Id, Slug, Text } from "./common.gen.js";

export const DecisionCites = z.strictObject({
  sources: z.array(Slug).default([]),
  rules: z.array(Id).default([]),
  bindings: z.array(Text).default([]),
});
export type DecisionCites = z.infer<typeof DecisionCites>;

/** Decision (entity, spec 16.1). */
export const Decision = z.strictObject({
  slug: Slug,
  title: Text,
  text: Text,
  rejected: z.array(Text).default([]),
  cites: DecisionCites.prefault({}),
  who: Text,
  date: Date,
  supersedes: Slug.nullable().default(null),
  superseded_by: Slug.nullable().default(null),
});
export type Decision = z.infer<typeof Decision>;
