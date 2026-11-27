// Written by gragas-generate from internal/host/assets/contract/schema/v1/document-sarif.schema.json; edits are lost, so edit the source and run task gen.

import { z } from "zod";
import { Contract, Id, Semver, Standing, Text, TextOrEmpty } from "./common.gen.js";

export const SarifDocumentRunsToolDriverRulesShortDescription = z.strictObject({
  text: Text,
});
export type SarifDocumentRunsToolDriverRulesShortDescription = z.infer<typeof SarifDocumentRunsToolDriverRulesShortDescription>;

export const SarifDocumentRunsToolDriverRules = z.strictObject({
  id: Id,
  shortDescription: SarifDocumentRunsToolDriverRulesShortDescription.nullable().default(null),
});
export type SarifDocumentRunsToolDriverRules = z.infer<typeof SarifDocumentRunsToolDriverRules>;

export const SarifDocumentRunsToolDriver = z.strictObject({
  name: Text,
  version: Semver.nullable().default(null),
  rules: z.array(SarifDocumentRunsToolDriverRules).default([]),
});
export type SarifDocumentRunsToolDriver = z.infer<typeof SarifDocumentRunsToolDriver>;

export const SarifDocumentRunsTool = z.strictObject({
  driver: SarifDocumentRunsToolDriver,
});
export type SarifDocumentRunsTool = z.infer<typeof SarifDocumentRunsTool>;

export const SarifDocumentRunsResultsLevel = z.enum(["error", "warning", "note", "none"]);
export type SarifDocumentRunsResultsLevel = z.infer<typeof SarifDocumentRunsResultsLevel>;

export const SarifDocumentRunsResultsMessage = z.strictObject({
  text: Text,
});
export type SarifDocumentRunsResultsMessage = z.infer<typeof SarifDocumentRunsResultsMessage>;

export const SarifDocumentRunsResultsLocationsPhysicalLocationArtifactLocation = z.strictObject({
  uri: Text,
});
export type SarifDocumentRunsResultsLocationsPhysicalLocationArtifactLocation = z.infer<typeof SarifDocumentRunsResultsLocationsPhysicalLocationArtifactLocation>;

export const SarifDocumentRunsResultsLocationsPhysicalLocationRegion = z.strictObject({
  startLine: z.number().int().min(1).nullable().default(null),
});
export type SarifDocumentRunsResultsLocationsPhysicalLocationRegion = z.infer<typeof SarifDocumentRunsResultsLocationsPhysicalLocationRegion>;

export const SarifDocumentRunsResultsLocationsPhysicalLocation = z.strictObject({
  artifactLocation: SarifDocumentRunsResultsLocationsPhysicalLocationArtifactLocation,
  region: SarifDocumentRunsResultsLocationsPhysicalLocationRegion.prefault({}),
});
export type SarifDocumentRunsResultsLocationsPhysicalLocation = z.infer<typeof SarifDocumentRunsResultsLocationsPhysicalLocation>;

export const SarifDocumentRunsResultsLocations = z.strictObject({
  physicalLocation: SarifDocumentRunsResultsLocationsPhysicalLocation,
});
export type SarifDocumentRunsResultsLocations = z.infer<typeof SarifDocumentRunsResultsLocations>;

export const SarifDocumentRunsResultsPropertiesGragas = z.strictObject({
  standing: Standing.nullable().default(null),
  fix: TextOrEmpty.default(""),
  notice: z.boolean().default(false),
});
export type SarifDocumentRunsResultsPropertiesGragas = z.infer<typeof SarifDocumentRunsResultsPropertiesGragas>;

export const SarifDocumentRunsResultsProperties = z.strictObject({
  gragas: SarifDocumentRunsResultsPropertiesGragas.prefault({}),
});
export type SarifDocumentRunsResultsProperties = z.infer<typeof SarifDocumentRunsResultsProperties>;

export const SarifDocumentRunsResults = z.strictObject({
  ruleId: Id,
  level: SarifDocumentRunsResultsLevel.default("error"),
  message: SarifDocumentRunsResultsMessage,
  locations: z.array(SarifDocumentRunsResultsLocations).default([]),
  properties: SarifDocumentRunsResultsProperties.prefault({}),
});
export type SarifDocumentRunsResults = z.infer<typeof SarifDocumentRunsResults>;

export const SarifDocumentRuns = z.strictObject({
  tool: SarifDocumentRunsTool,
  results: z.array(SarifDocumentRunsResults),
});
export type SarifDocumentRuns = z.infer<typeof SarifDocumentRuns>;

/** SARIF 2.1.0 with the engine's extension (document). */
export const SarifDocument = z.strictObject({
  contract: Contract,
  version: z.literal("2.1.0"),
  $schema: z.string().nullable().default(null),
  runs: z.array(SarifDocumentRuns),
});
export type SarifDocument = z.infer<typeof SarifDocument>;
