// Written by gragas-generate from internal/host/assets/contract/schema/v1/entity-rule-map.schema.json; edits are lost, so edit the source and run task gen.

import { z } from "zod";
import { Id, Mark, Reason, Text } from "./common.gen.js";

export const RuleMapRows = z.strictObject({
  left: Id.nullable().default(null),
  arrived: Id.nullable().default(null),
  mark: Mark,
  reason: Reason.nullable().default(null),
});
export type RuleMapRows = z.infer<typeof RuleMapRows>;

/** Rule map toward one mapped rulebook (entity, spec 16.1). */
export const RuleMap = z.strictObject({
  rulebook: Text,
  rows: z.array(RuleMapRows),
});
export type RuleMap = z.infer<typeof RuleMap>;
