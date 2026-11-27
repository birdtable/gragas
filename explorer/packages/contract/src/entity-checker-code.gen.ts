// Written by gragas-generate from internal/host/assets/contract/schema/v1/entity-checker-code.schema.json; edits are lost, so edit the source and run task gen.

import { z } from "zod";
import { Digest, Slug, Text, TextOrEmpty } from "./common.gen.js";

/** Checker-code artifact (entity, spec 16.1). */
export const CheckerCode = z.strictObject({
  name: Slug,
  publisher: Text,
  digest: Digest,
  tool: Text,
  tests_present: z.boolean(),
  reviewed_by: z.array(Text).default([]),
  licence: TextOrEmpty.default(""),
  by_reference: z.boolean().default(false),
});
export type CheckerCode = z.infer<typeof CheckerCode>;
