// Written by gragas-generate from internal/host/assets/contract/schema/v1/entity-binding.schema.json; edits are lost, so edit the source and run task gen.

import { z } from "zod";
import { Id, RunPoint, Rung, Severity, Slug, TargetType, Text, TextOrEmpty } from "./common.gen.js";
import { ProofResult } from "./entity-proof-result.gen.js";

/** Binding of a rule to a tool key (entity, spec 16.1). */
export const Binding = z.strictObject({
  rule: Id,
  tool: Text,
  key: Text,
  target_type: TargetType,
  value: z.unknown().nullable().default(null),
  options: z.record(z.string(), z.unknown()).default({}),
  severity: Severity,
  rung: Rung,
  run_point: RunPoint,
  remedy: TextOrEmpty.default(""),
  pair: z.number().int().min(1).nullable().default(null),
  pair_path: TextOrEmpty.default(""),
  proof: ProofResult.nullable().default(null),
  tightens: Id.nullable().default(null),
  checker: Slug.nullable().default(null),
});
export type Binding = z.infer<typeof Binding>;
