// Written by gragas-generate from internal/host/assets/contract/schema/v1/entity-rulebook.schema.json; edits are lost, so edit the source and run task gen.

import { z } from "zod";
import { DigestOrEmpty, Language, Semver, Slug, Text, TextOrEmpty, Url } from "./common.gen.js";

export const RulebookKind = z.enum(["base", "overlay", "companion", "fork", "archived"]);
export type RulebookKind = z.infer<typeof RulebookKind>;

export const RulebookLicences = z.strictObject({
  rules: Text,
  checker_code: TextOrEmpty.default(""),
  presets: TextOrEmpty.default(""),
});
export type RulebookLicences = z.infer<typeof RulebookLicences>;

export const RulebookExtends = z.strictObject({
  rulebook: Text,
  version: Semver,
  content_hash: DigestOrEmpty.default(""),
});
export type RulebookExtends = z.infer<typeof RulebookExtends>;

export const RulebookImports = z.strictObject({
  rulebook: Text,
  version: Semver,
  parts: z.array(Slug),
  content_hash: DigestOrEmpty.default(""),
});
export type RulebookImports = z.infer<typeof RulebookImports>;

export const RulebookGovernanceContribution = z.enum(["open", "proposals", "closed"]);
export type RulebookGovernanceContribution = z.infer<typeof RulebookGovernanceContribution>;

export const RulebookGovernance = z.strictObject({
  contribution: RulebookGovernanceContribution,
  decision_required: z.boolean(),
  trial_releases: z.number().int().min(0),
  review_days: z.number().int().min(1),
});
export type RulebookGovernance = z.infer<typeof RulebookGovernance>;

/** Rulebook: the identity a document carries (entity, spec 16.1). */
export const Rulebook = z.strictObject({
  name: Slug,
  version: Semver,
  kind: RulebookKind,
  licences: RulebookLicences,
  language: Language,
  docs_url: Url.nullable().default(null),
  explorer_url: Url.nullable().default(null),
  extends: RulebookExtends.nullable().default(null),
  imports: z.array(RulebookImports).default([]),
  owners: z.array(Text),
  governance: RulebookGovernance,
  format: z.number().int().min(1),
  successor: Slug.nullable().default(null),
});
export type Rulebook = z.infer<typeof Rulebook>;
