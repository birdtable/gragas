// Written by gragas-generate from internal/host/assets/contract/schema/v1/entity-finding.schema.json; edits are lost, so edit the source and run task gen.

import { z } from "zod";
import { Id, Slug, Text, TextOrEmpty } from "./common.gen.js";

/** Finding or notice (entity, spec 16.1). */
export const Finding = z.strictObject({
  kind: Slug,
  entity: Text,
  location: TextOrEmpty.default(""),
  message: Text,
  fix: TextOrEmpty.default(""),
  rule: Id.nullable().default(null),
  refusal: z.boolean().default(false),
  notice: z.boolean().default(false),
});
export type Finding = z.infer<typeof Finding>;
