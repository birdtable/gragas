// Written by gragas-generate from internal/host/assets/contract/schema/v1/entity-upstream-set.schema.json; edits are lost, so edit the source and run task gen.

import { z } from "zod";
import { Slug, Text } from "./common.gen.js";

export const UpstreamSetSets = z.strictObject({
  preset: Text,
  entries: z.array(Text),
});
export type UpstreamSetSets = z.infer<typeof UpstreamSetSets>;

/** The flattened upstream set of one tool (entity). */
export const UpstreamSet = z.strictObject({
  version: z.number().int().min(1),
  tool: Slug,
  sets: z.array(UpstreamSetSets),
  observed: z.array(Text),
});
export type UpstreamSet = z.infer<typeof UpstreamSet>;
