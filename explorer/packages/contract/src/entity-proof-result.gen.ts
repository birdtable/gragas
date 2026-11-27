// Written by gragas-generate from internal/host/assets/contract/schema/v1/entity-proof-result.schema.json; edits are lost, so edit the source and run task gen.

import { z } from "zod";
import { DigestOrEmpty, Result, Rung, Text, TextOrEmpty } from "./common.gen.js";

export const ProofResultRunLevel = z.enum(["fixture", "pair", "project"]);
export type ProofResultRunLevel = z.infer<typeof ProofResultRunLevel>;

export const ProofResultReason = z.enum(["no pair", "tool missing", "limit hit", "sandbox missing"]);
export type ProofResultReason = z.infer<typeof ProofResultReason>;

/** Proof result of one binding under one tool version (entity, spec 16.1). */
export const ProofResult = z.strictObject({
  binding: Text,
  tool_version: Text,
  result: Result,
  rung: Rung.default(0),
  run_level: ProofResultRunLevel.default("fixture"),
  adapter_version: TextOrEmpty.default(""),
  reason: ProofResultReason.default("no pair"),
  inputs: DigestOrEmpty.default(""),
  tool: TextOrEmpty.default(""),
});
export type ProofResult = z.infer<typeof ProofResult>;
