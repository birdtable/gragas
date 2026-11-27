// Written by gragas-generate from internal/host/assets/contract/schema/v1/entity-translation.schema.json; edits are lost, so edit the source and run task gen.

import { z } from "zod";
import { Digest, Language, Slug, Text } from "./common.gen.js";

/** Translation of one field of one entity (entity, spec 16.1). */
export const Translation = z.strictObject({
  entity: Text,
  field: Slug,
  language: Language,
  text: Text,
  authoritative_hash: Digest,
  stale: z.boolean(),
});
export type Translation = z.infer<typeof Translation>;
