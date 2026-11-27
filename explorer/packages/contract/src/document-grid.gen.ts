// Written by gragas-generate from internal/host/assets/contract/schema/v1/document-grid.schema.json; edits are lost, so edit the source and run task gen.

import { z } from "zod";
import { Contract } from "./common.gen.js";
import { ProofResult } from "./entity-proof-result.gen.js";
import { TestedSet } from "./entity-tested-set.gen.js";

/** The proof grid, binding by tool version (document, spec S57). */
export const GridDocument = z.strictObject({
  contract: Contract,
  cells: z.array(ProofResult),
  tested_set: TestedSet.nullable().default(null),
});
export type GridDocument = z.infer<typeof GridDocument>;
