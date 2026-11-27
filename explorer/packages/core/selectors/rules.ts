// The rules table's rows: each bundle rule joined by id to the
// report's entries for it. A report has one section per package (spec S48), so
// a rule can stand in several sections: the row keeps every one, and says when
// they disagree. Pure over the parsed documents.
import type { BundleDocument, ReportDocument } from "@gragas/contract";
import type * as z from "zod";
import { groupById, indexById, joinById } from "./by-id.ts";
import { otherRelease, type OtherRelease } from "./release.ts";

type Bundle = z.output<typeof BundleDocument>;
type Report = z.output<typeof ReportDocument>;
type Rule = Bundle["rules"][number];
type PerRule = Report["sections"][number]["per_rule"][number];

export interface SectionEntry {
  /** The section's manifest digest: the package it speaks for. */
  readonly manifest: string;
  readonly entry: PerRule;
}

export interface RuleRow {
  readonly rule: Rule;
  /** The rule's entry in every section that names it, in section order. */
  readonly sections: readonly SectionEntry[];
  /** Two sections name the rule with a different observation or standing. */
  readonly disagree: boolean;
  /** The report speaks for another release and has no entry for this rule. */
  readonly differs: boolean;
}

export interface RuleTable {
  readonly rows: readonly RuleRow[];
  /** Rules the report names that the bundle does not hold. */
  readonly reportOnly: readonly string[];
  readonly banner: OtherRelease | null;
}

function disagrees(found: readonly SectionEntry[]): boolean {
  return found.some(
    ({ entry }) =>
      entry.observation !== found[0]?.entry.observation ||
      entry.standing !== found[0].entry.standing,
  );
}

export function ruleTable(bundle: Bundle, report: Report | null): RuleTable {
  const banner = report === null ? null : otherRelease(bundle, report);
  const entries = groupById(
    (report?.sections ?? []).flatMap((section) =>
      section.per_rule.map((entry) => ({ manifest: section.manifest, entry })),
    ),
    ({ entry }) => entry.rule,
  );
  const rows = joinById(bundle.rules, (rule) => rule.id, entries).map(
    ({ row, match }) => {
      const sections = match ?? [];
      return {
        rule: row,
        sections,
        disagree: disagrees(sections),
        differs: banner !== null && match === undefined,
      };
    },
  );
  const held = indexById(bundle.rules, (rule) => rule.id);
  const reportOnly = [...entries.keys()].filter((id) => !held.byId.has(id));
  return { rows, reportOnly, banner };
}
