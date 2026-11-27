// Written by gragas-generate from internal/host/assets/contract/schema/v1/document-plan.schema.json; edits are lost, so edit the source and run task gen.

import { z } from "zod";
import { Contract } from "./common.gen.js";
import { Plan } from "./entity-plan.gen.js";

/** A plan as a document (spec 15.3). */
export const PlanDocument = z.strictObject({
  contract: Contract,
  plan: Plan,
});
export type PlanDocument = z.infer<typeof PlanDocument>;
