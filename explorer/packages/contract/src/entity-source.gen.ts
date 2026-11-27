// Written by gragas-generate from internal/host/assets/contract/schema/v1/entity-source.schema.json; edits are lost, so edit the source and run task gen.

import { z } from "zod";
import { Slug, Text, TextOrEmpty } from "./common.gen.js";

export const SourceKind = z.enum(["standard", "guide", "study", "incident", "decision record", "vendor document", "own decision"]);
export type SourceKind = z.infer<typeof SourceKind>;

/** Source (entity, spec 16.1). */
export const Source = z.strictObject({
  slug: Slug,
  kind: SourceKind,
  title: Text,
  locator: TextOrEmpty.default(""),
  attribution: TextOrEmpty.default(""),
});
export type Source = z.infer<typeof Source>;
