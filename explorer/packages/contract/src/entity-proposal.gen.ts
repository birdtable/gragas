// Written by gragas-generate from internal/host/assets/contract/schema/v1/entity-proposal.schema.json; edits are lost, so edit the source and run task gen.

import { z } from "zod";
import { Date, Slug, Text, TextOrEmpty, Url } from "./common.gen.js";

export const ProposalChange = z.strictObject({
  anatomy: Text,
  pair: TextOrEmpty.default(""),
  binding: TextOrEmpty.default(""),
  sources: z.array(Slug).default([]),
});
export type ProposalChange = z.infer<typeof ProposalChange>;

export const ProposalEvidenceKind = z.enum(["waiver-pattern", "incident-example", "agent-miss"]);
export type ProposalEvidenceKind = z.infer<typeof ProposalEvidenceKind>;

export const ProposalEvidence = z.strictObject({
  kind: ProposalEvidenceKind,
  text: Text,
});
export type ProposalEvidence = z.infer<typeof ProposalEvidence>;

export const ProposalState = z.enum(["open", "accepted", "declined", "withdrawn"]);
export type ProposalState = z.infer<typeof ProposalState>;

/** Proposal (entity, spec 16.1). */
export const Proposal = z.strictObject({
  slug: Slug,
  target: Text,
  change: ProposalChange,
  evidence: z.array(ProposalEvidence).default([]),
  state: ProposalState,
  link: Url.nullable().default(null),
  opened: Date,
});
export type Proposal = z.infer<typeof Proposal>;
