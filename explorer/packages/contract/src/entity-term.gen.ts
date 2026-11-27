// Written by gragas-generate from internal/host/assets/contract/schema/v1/entity-term.schema.json; edits are lost, so edit the source and run task gen.

import { z } from "zod";
import { Slug, Text, TextOrEmpty } from "./common.gen.js";
import { Translation } from "./entity-translation.gen.js";

/** Term (entity, spec 16.1). */
export const Term = z.strictObject({
  slug: Slug,
  definition: Text,
  literal: TextOrEmpty.default(""),
  translations: z.array(Translation).default([]),
});
export type Term = z.infer<typeof Term>;
