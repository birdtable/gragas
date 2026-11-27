// Written by gragas-generate from internal/host/assets/contract/schema/v1/document-bundle.schema.json; edits are lost, so edit the source and run task gen.

import { z } from "zod";
import { Contract, Id, Text, Versions } from "./common.gen.js";
import { Binding } from "./entity-binding.gen.js";
import { ChangelogEntry } from "./entity-changelog-entry.gen.js";
import { CheckerCode } from "./entity-checker-code.gen.js";
import { Decision } from "./entity-decision.gen.js";
import { Exclusion } from "./entity-exclusion.gen.js";
import { Finding } from "./entity-finding.gen.js";
import { Health } from "./entity-health.gen.js";
import { HistoryEntry } from "./entity-history-entry.gen.js";
import { Preset } from "./entity-preset.gen.js";
import { ProofResult } from "./entity-proof-result.gen.js";
import { Release } from "./entity-release.gen.js";
import { RenderingManifest } from "./entity-rendering-manifest.gen.js";
import { RuleMap } from "./entity-rule-map.gen.js";
import { Rule } from "./entity-rule.gen.js";
import { Rulebook } from "./entity-rulebook.gen.js";
import { Source } from "./entity-source.gen.js";
import { Structure } from "./entity-structure.gen.js";
import { Term } from "./entity-term.gen.js";
import { TestedSet } from "./entity-tested-set.gen.js";

export const BundleDocumentAdaptersUsed = z.strictObject({
  tool: Text,
  adapter_version: Text,
  waiver_syntax: Text.nullable().default(null),
});
export type BundleDocumentAdaptersUsed = z.infer<typeof BundleDocumentAdaptersUsed>;

export const BundleDocumentCoverage = z.strictObject({
  rules_to_bindings: z.array(Id).default([]),
  bindings_to_rules: z.array(Text).default([]),
});
export type BundleDocumentCoverage = z.infer<typeof BundleDocumentCoverage>;

/** The rulebook bundle (document, spec 16.9). */
export const BundleDocument = z.strictObject({
  contract: Contract,
  versions: Versions,
  release: Release,
  released: z.boolean(),
  identity: Rulebook,
  structure: Structure,
  terms: z.array(Term),
  rules: z.array(Rule),
  exclusions: z.array(Exclusion).default([]),
  bindings: z.array(Binding),
  adapters_used: z.array(BundleDocumentAdaptersUsed).default([]),
  presets: z.array(Preset).default([]),
  checker_code: z.array(CheckerCode).default([]),
  proof_grid: z.array(ProofResult).default([]),
  coverage: BundleDocumentCoverage.prefault({}),
  conflicts: z.array(Finding).default([]),
  decisions: z.array(Decision),
  sources: z.array(Source),
  history: z.array(HistoryEntry),
  changelog: z.array(ChangelogEntry),
  tested_set: TestedSet,
  health: Health,
  rule_map: z.array(RuleMap).default([]),
  rendering_manifest: z.array(RenderingManifest).default([]),
});
export type BundleDocument = z.infer<typeof BundleDocument>;
