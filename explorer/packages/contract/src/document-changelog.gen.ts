// Written by gragas-generate from internal/host/assets/contract/schema/v1/document-changelog.schema.json; edits are lost, so edit the source and run task gen.

import { z } from "zod";
import { Bump, Contract, Date, Semver, Slug, TextOrEmpty } from "./common.gen.js";
import { ChangelogEntry } from "./entity-changelog-entry.gen.js";

export const ChangelogDocumentReleases = z.strictObject({
  version: Semver,
  date: Date,
  bump: Bump.nullable().default(null),
  release_note: TextOrEmpty.default(""),
  entries: z.array(ChangelogEntry),
});
export type ChangelogDocumentReleases = z.infer<typeof ChangelogDocumentReleases>;

/** The changelog across releases (document). */
export const ChangelogDocument = z.strictObject({
  contract: Contract,
  rulebook: Slug,
  releases: z.array(ChangelogDocumentReleases),
});
export type ChangelogDocument = z.infer<typeof ChangelogDocument>;
