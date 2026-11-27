// Written by gragas-generate from internal/host/assets/contract/schema/v1/document-proposal.schema.json; edits are lost, so edit the source and run task gen.

import { z } from "zod";
import { Contract } from "./common.gen.js";
import { Proposal } from "./entity-proposal.gen.js";

/** A proposal as a document (spec 16.1). */
export const ProposalDocument = z.strictObject({
  contract: Contract,
  proposal: Proposal,
});
export type ProposalDocument = z.infer<typeof ProposalDocument>;
