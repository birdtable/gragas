// Written by gragas-generate from internal/host/assets/contract/schema/v1/entity-structure.schema.json; edits are lost, so edit the source and run task gen.

import { z } from "zod";
import { Slug, Text, TextOrEmpty } from "./common.gen.js";

export const StructureLayers = z.strictObject({
  slug: Slug,
  name: Text,
  order: z.number().int().min(0),
});
export type StructureLayers = z.infer<typeof StructureLayers>;

export const StructurePartsConcerns = z.strictObject({
  slug: Slug,
  name: Text,
  order: z.number().int().min(0),
});
export type StructurePartsConcerns = z.infer<typeof StructurePartsConcerns>;

export const StructureParts = z.strictObject({
  slug: Slug,
  name: Text,
  layer: Slug,
  order: z.number().int().min(0),
  guide: TextOrEmpty.default(""),
  owners: z.array(Text).default([]),
  aliases: z.array(Slug).default([]),
  concerns: z.array(StructurePartsConcerns).default([]),
});
export type StructureParts = z.infer<typeof StructureParts>;

export const StructureKinds = z.strictObject({
  slug: Slug,
  name: Text,
  description: TextOrEmpty.default(""),
});
export type StructureKinds = z.infer<typeof StructureKinds>;

/** Layers, parts, concerns and kinds (entity, spec 16.1). */
export const Structure = z.strictObject({
  layers: z.array(StructureLayers),
  parts: z.array(StructureParts),
  kinds: z.array(StructureKinds),
});
export type Structure = z.infer<typeof Structure>;
