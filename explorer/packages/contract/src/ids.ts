// The hand-written layer beside the generated schemas: ids that
// cannot be mixed up. Each brand narrows the generated pattern, so a rule id
// is not a source id to the compiler (ts.type-safety.6), and the only way to
// get one is to parse it.
import type * as z from "zod";
import { Id, Semver, Slug } from "./common.gen.js";

/** `go.errors.1`: part, concern, number. */
export const RuleId = Id.brand<"RuleId">();
export type RuleId = z.infer<typeof RuleId>;

/** A binding is named by the rule it binds, so its id has the rule's shape. */
export const BindingId = Id.brand<"BindingId">();
export type BindingId = z.infer<typeof BindingId>;

export const DecisionId = Slug.brand<"DecisionId">();
export type DecisionId = z.infer<typeof DecisionId>;

export const SourceId = Slug.brand<"SourceId">();
export type SourceId = z.infer<typeof SourceId>;

export const TermId = Slug.brand<"TermId">();
export type TermId = z.infer<typeof TermId>;

/** A release is named by its version. */
export const ReleaseId = Semver.brand<"ReleaseId">();
export type ReleaseId = z.infer<typeof ReleaseId>;
