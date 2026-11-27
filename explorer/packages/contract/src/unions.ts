// Closed unions the views switch on (ts.type-safety.7). Each is the generated
// enum under a name the views read, so a new member in the schema breaks every
// switch that does not name it: `exhaustive` is the default arm of such a
// switch and only compiles when no member is left.
import type * as z from "zod";
import { Mark, Result, Standing, Status } from "./common.gen.js";
import { ProofResultRunLevel } from "./entity-proof-result.gen.js";
import { ReportDocumentSectionsPerRuleObservation } from "./document-report.gen.js";

export type RuleStatus = z.infer<typeof Status>;
export type RuleStanding = z.infer<typeof Standing>;
export type RuleMark = z.infer<typeof Mark>;
export type Observation = z.infer<
  typeof ReportDocumentSectionsPerRuleObservation
>;
export type ProofOutcome = z.infer<typeof Result>;
export type RunLevel = z.infer<typeof ProofResultRunLevel>;

/** Every member of each closed union, in the schema's order. */
export const closedUnions = {
  status: Status.options,
  standing: Standing.options,
  mark: Mark.options,
  observation: ReportDocumentSectionsPerRuleObservation.options,
  proofOutcome: Result.options,
  level: ProofResultRunLevel.options,
} as const;

/** The default arm of a switch over a closed union; a missed member is a compile error. */
export function exhaustive(value: never): never {
  throw new Error(
    `a member of a closed union was not handled: ${String(value)}`,
  );
}
