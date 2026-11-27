// The six documents the contract defines, by the name the field list and the
// fixtures use for them.
import { BundleDocument } from "./document-bundle.gen.js";
import { ChangelogDocument } from "./document-changelog.gen.js";
import { PlanDocument } from "./document-plan.gen.js";
import { ProposalDocument } from "./document-proposal.gen.js";
import { ReportDocument } from "./document-report.gen.js";
import { SarifDocument } from "./document-sarif.gen.js";

export const documents = {
  bundle: BundleDocument,
  report: ReportDocument,
  changelog: ChangelogDocument,
  sarif: SarifDocument,
  plan: PlanDocument,
  proposal: ProposalDocument,
} as const;

export type DocumentName = keyof typeof documents;

export const documentNames = [
  "bundle",
  "report",
  "changelog",
  "sarif",
  "plan",
  "proposal",
] as const satisfies readonly DocumentName[];
