// Written by gragas-generate from internal/host/assets/contract/schema/v1/entity-rule.schema.json; edits are lost, so edit the source and run task gen.

import { z } from "zod";
import { Date, Id, Origin, Reason, Semver, Slug, Standing, StandingMark, Status, Strength, TargetType, Text, TextOrEmpty } from "./common.gen.js";
import { Translation } from "./entity-translation.gen.js";

export const RuleScope = z.strictObject({
  parts: z.array(Slug).default([]),
  paths: z.array(Text).default([]),
  kinds: z.array(Slug).default([]),
});
export type RuleScope = z.infer<typeof RuleScope>;

export const RuleExemptions = z.strictObject({
  path: TextOrEmpty.default(""),
  kind: Slug.nullable().default(null),
  reason: Reason,
});
export type RuleExemptions = z.infer<typeof RuleExemptions>;

export const RulePairs = z.strictObject({
  number: z.number().int().min(1),
  target_type: TargetType,
  good: Text.nullable().default(null),
  bad: Text.nullable().default(null),
});
export type RulePairs = z.infer<typeof RulePairs>;

export const RuleRelationsKind = z.enum(["pairs-with", "conflicts-with", "refines", "see-also"]);
export type RuleRelationsKind = z.infer<typeof RuleRelationsKind>;

export const RuleRelations = z.strictObject({
  kind: RuleRelationsKind,
  rule: Id,
});
export type RuleRelations = z.infer<typeof RuleRelations>;

/** Rule (entity, spec 16.1). */
export const Rule = z.strictObject({
  id: Id,
  aliases: z.array(Id).default([]),
  statement: Text,
  strength: Strength,
  status: Status,
  standing: Standing,
  marks: z.array(StandingMark).default([]),
  trial_shipped: z.array(Semver).default([]),
  scope: RuleScope.prefault({}),
  exemptions: z.array(RuleExemptions).default([]),
  reasoning: TextOrEmpty.default(""),
  reasoning_decision: Slug.nullable().default(null),
  pros: z.array(Text).default([]),
  cons: z.array(Text).default([]),
  pairs: z.array(RulePairs).default([]),
  owner: TextOrEmpty.default(""),
  review_by: Date.nullable().default(null),
  zero_by: Date.nullable().default(null),
  relations: z.array(RuleRelations).default([]),
  origin: Origin,
  sources: z.array(Slug).default([]),
  bindings: z.array(Text).default([]),
  translations: z.array(Translation).default([]),
  replaced_by: Id.nullable().default(null),
  anchor: TextOrEmpty.default(""),
});
export type Rule = z.infer<typeof Rule>;
