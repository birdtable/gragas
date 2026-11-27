// Written by gragas-generate from internal/host/assets/contract/schema/v1/entity-changelog-entry.schema.json; edits are lost, so edit the source and run task gen.

import { z } from "zod";
import { Bump, Date, Semver, Text, TextOrEmpty, Url } from "./common.gen.js";

export const ChangelogEntryChange = z.enum(["added", "changed", "weakened", "tightened", "retired", "restored", "replaced"]);
export type ChangelogEntryChange = z.infer<typeof ChangelogEntryChange>;

/** Changelog entry (entity, spec 16.1). */
export const ChangelogEntry = z.strictObject({
  release: Semver,
  entity: Text,
  change: ChangelogEntryChange,
  bump: Bump.nullable().default(null),
  date: Date.nullable().default(null),
  sentence: TextOrEmpty.default(""),
  consumer_action: TextOrEmpty.default(""),
  remedy: TextOrEmpty.default(""),
  diff: Url.nullable().default(null),
});
export type ChangelogEntry = z.infer<typeof ChangelogEntry>;
