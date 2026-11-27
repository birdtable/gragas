// Written by gragas-generate from internal/host/assets/contract/schema/v1/document-report.schema.json; edits are lost, so edit the source and run task gen.

import { z } from "zod";
import { Contract, Digest, Id, Semver, Slug, Standing, Text, TextOrEmpty, Timestamp, Versions } from "./common.gen.js";
import { Baseline } from "./entity-baseline.gen.js";
import { Release } from "./entity-release.gen.js";
import { Waiver } from "./entity-waiver.gen.js";

export const ReportDocumentPin = z.strictObject({
  rulebook: Text,
  version: Semver,
});
export type ReportDocumentPin = z.infer<typeof ReportDocumentPin>;

export const ReportDocumentSectionsPerRuleObservation = z.enum(["held", "not held", "not observed"]);
export type ReportDocumentSectionsPerRuleObservation = z.infer<typeof ReportDocumentSectionsPerRuleObservation>;

export const ReportDocumentSectionsPerRuleProvenance = z.strictObject({
  layer: Slug.nullable().default(null),
  decision: Slug.nullable().default(null),
  switch: Slug.nullable().default(null),
});
export type ReportDocumentSectionsPerRuleProvenance = z.infer<typeof ReportDocumentSectionsPerRuleProvenance>;

export const ReportDocumentSectionsPerRule = z.strictObject({
  rule: Id,
  standing: Standing,
  marks: z.array(Slug).default([]),
  observation: ReportDocumentSectionsPerRuleObservation.default("held"),
  provenance: ReportDocumentSectionsPerRuleProvenance.prefault({}),
  bindings: z.array(Text).default([]),
});
export type ReportDocumentSectionsPerRule = z.infer<typeof ReportDocumentSectionsPerRule>;

export const ReportDocumentSections = z.strictObject({
  manifest: Digest,
  name: TextOrEmpty.default(""),
  per_rule: z.array(ReportDocumentSectionsPerRule),
});
export type ReportDocumentSections = z.infer<typeof ReportDocumentSections>;

export const ReportDocumentDriftKind = z.enum(["weaker", "tighter", "absent", "unmapped", "edited copy"]);
export type ReportDocumentDriftKind = z.infer<typeof ReportDocumentDriftKind>;

export const ReportDocumentDrift = z.strictObject({
  entry: Text,
  kind: ReportDocumentDriftKind,
});
export type ReportDocumentDrift = z.infer<typeof ReportDocumentDrift>;

export const ReportDocumentMismatchesWhat = z.enum(["tool", "preset", "adapter", "engine"]);
export type ReportDocumentMismatchesWhat = z.infer<typeof ReportDocumentMismatchesWhat>;

export const ReportDocumentMismatches = z.strictObject({
  what: ReportDocumentMismatchesWhat,
  found: Text,
  expected: Text,
});
export type ReportDocumentMismatches = z.infer<typeof ReportDocumentMismatches>;

export const ReportDocumentRenderingsState = z.enum(["stale", "edited"]);
export type ReportDocumentRenderingsState = z.infer<typeof ReportDocumentRenderingsState>;

export const ReportDocumentRenderings = z.strictObject({
  path: Text,
  state: ReportDocumentRenderingsState,
});
export type ReportDocumentRenderings = z.infer<typeof ReportDocumentRenderings>;

export const ReportDocumentWiring = z.strictObject({
  line: Text,
  present: z.boolean(),
});
export type ReportDocumentWiring = z.infer<typeof ReportDocumentWiring>;

export const ReportDocumentTrustSignature = z.enum(["verified", "unsigned-accepted"]);
export type ReportDocumentTrustSignature = z.infer<typeof ReportDocumentTrustSignature>;

export const ReportDocumentTrustReleaseState = z.enum(["current", "withdrawn", "archived"]);
export type ReportDocumentTrustReleaseState = z.infer<typeof ReportDocumentTrustReleaseState>;

export const ReportDocumentTrust = z.strictObject({
  signature: ReportDocumentTrustSignature,
  digests_matched: z.boolean(),
  release_state: ReportDocumentTrustReleaseState,
});
export type ReportDocumentTrust = z.infer<typeof ReportDocumentTrust>;

export const ReportDocumentLastChangeKind = z.enum(["upgrade", "switch"]);
export type ReportDocumentLastChangeKind = z.infer<typeof ReportDocumentLastChangeKind>;

export const ReportDocumentLastChange = z.strictObject({
  kind: ReportDocumentLastChangeKind,
  when: Timestamp,
});
export type ReportDocumentLastChange = z.infer<typeof ReportDocumentLastChange>;

/** The project report (document, spec 16.8): carries no project name, path, remote or author. */
export const ReportDocument = z.strictObject({
  contract: Contract,
  versions: Versions,
  pin: ReportDocumentPin,
  release: Release,
  run_time: Timestamp,
  lock_digest: Digest,
  sections: z.array(ReportDocumentSections),
  waivers: z.array(Waiver).default([]),
  baselines: z.array(Baseline).default([]),
  drift: z.array(ReportDocumentDrift).default([]),
  mismatches: z.array(ReportDocumentMismatches).default([]),
  renderings: z.array(ReportDocumentRenderings).default([]),
  wiring: z.array(ReportDocumentWiring).default([]),
  trust: ReportDocumentTrust,
  last_change: ReportDocumentLastChange.nullable().default(null),
});
export type ReportDocument = z.infer<typeof ReportDocument>;
