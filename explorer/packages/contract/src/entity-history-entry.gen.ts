// Written by gragas-generate from internal/host/assets/contract/schema/v1/entity-history-entry.schema.json; edits are lost, so edit the source and run task gen.

import { z } from "zod";
import { Digest, Slug, Status, Text, Timestamp } from "./common.gen.js";

/** History entry (entity, spec 16.1). */
export const HistoryEntry = z.strictObject({
  number: z.number().int().min(1),
  kind: Slug,
  entity: Text,
  before: z.record(z.string(), z.unknown()).default({}),
  after: z.record(z.string(), z.unknown()),
  who: Text,
  when: Timestamp,
  plan: Digest,
  command: Text.nullable().default(null),
  decision: Slug.nullable().default(null),
  status: Status.nullable().default(null),
});
export type HistoryEntry = z.infer<typeof HistoryEntry>;
