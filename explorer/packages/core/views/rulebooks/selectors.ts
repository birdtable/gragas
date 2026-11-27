// The rulebooks view's selectors: the chain this bundle belongs to,
// as boxes with pins; its imports by part; the rule maps
// toward each mapped rulebook; governance and licences per kind; an archived
// rulebook and its successor; and the provenance of each rule's layer for the
// rules table's "changed by" column. They read the one parsed bundle and take
// nothing else: gragas never knows which projects pin the chain, and the
// withdrawn list is a separate signed file, so neither is here.
import type { BundleDocument } from "@gragas/contract";
import type * as z from "zod";
import { dash, dashable, type Dashable } from "../../messages/index.ts";

type Bundle = z.output<typeof BundleDocument>;

export interface ChainBox {
  readonly role: "parent" | "this" | "import";
  readonly name: string;
  /** The pin: the version this bundle was built against. */
  readonly version: string;
  /** For an import, the parts taken; none for the others. */
  readonly parts: readonly string[];
}

export interface ImportRow {
  readonly rulebook: string;
  readonly version: string;
  readonly parts: readonly string[];
}

/** The counts of one rule map: rows kept, tighter, weaker, gone and new. */
export interface RuleMapRow {
  readonly rulebook: string;
  readonly kept: number;
  readonly tighter: number;
  readonly weaker: number;
  readonly gone: number;
  readonly new: number;
  /** Rows with a rule of this rulebook that arrived there, and rows with a rule that left this one. */
  readonly arrived: number;
  readonly left: number;
}

export interface LicenceRow {
  readonly kind: "rules" | "checker-code" | "presets";
  readonly licence: Dashable<string>;
}

export interface ProvenanceRow {
  readonly rule: string;
  readonly origin: "authored" | "imported" | "migrated";
  /** The imports whose parts the rule is scoped to, for an imported rule. */
  readonly via: Dashable<readonly string[]>;
}

export interface RulebooksView {
  readonly name: string;
  readonly version: string;
  readonly kind: Bundle["identity"]["kind"];
  readonly chain: readonly ChainBox[];
  readonly imports: readonly ImportRow[];
  readonly ruleMaps: readonly RuleMapRow[];
  readonly governance: {
    readonly contribution: string;
    readonly decisionRequired: boolean;
    readonly trialReleases: number;
    readonly reviewDays: number;
  };
  readonly licences: readonly LicenceRow[];
  /** For an archived rulebook: the successor its identity names, or a dash when it names none. */
  readonly archived: { readonly successor: Dashable<string> } | null;
  readonly provenance: readonly ProvenanceRow[];
}

function ruleMapRow(map: Bundle["rule_map"][number]): RuleMapRow {
  const count = (test: (row: (typeof map.rows)[number]) => boolean): number =>
    map.rows.filter(test).length;
  return {
    rulebook: map.rulebook,
    kept: count((r) => r.mark === "same"),
    tighter: count((r) => r.mark === "tighter"),
    weaker: count((r) => r.mark === "weaker"),
    gone: count((r) => r.left !== null && r.arrived === null),
    new: count((r) => r.left === null && r.arrived !== null),
    arrived: count((r) => r.arrived !== null),
    left: count((r) => r.left !== null),
  };
}

export function rulebooksView(bundle: Bundle): RulebooksView {
  const { identity } = bundle;
  const chain: ChainBox[] = [
    ...(identity.extends === null
      ? []
      : [
          {
            role: "parent" as const,
            name: identity.extends.rulebook,
            version: identity.extends.version,
            parts: [],
          },
        ]),
    {
      role: "this" as const,
      name: identity.name,
      version: identity.version,
      parts: [],
    },
    ...identity.imports.map((imp) => ({
      role: "import" as const,
      name: imp.rulebook,
      version: imp.version,
      parts: [...imp.parts],
    })),
  ];
  return {
    name: identity.name,
    version: identity.version,
    kind: identity.kind,
    chain,
    imports: identity.imports.map((imp) => ({
      rulebook: imp.rulebook,
      version: imp.version,
      parts: [...imp.parts],
    })),
    ruleMaps: bundle.rule_map.map(ruleMapRow),
    governance: {
      contribution: identity.governance.contribution,
      decisionRequired: identity.governance.decision_required,
      trialReleases: identity.governance.trial_releases,
      reviewDays: identity.governance.review_days,
    },
    licences: [
      {
        kind: "rules",
        licence: dashable(identity.licences.rules, "absent.not-in-bundle"),
      },
      {
        kind: "checker-code",
        licence: dashable(
          identity.licences.checker_code === ""
            ? null
            : identity.licences.checker_code,
          "absent.none-recorded",
        ),
      },
      {
        kind: "presets",
        licence: dashable(
          identity.licences.presets === "" ? null : identity.licences.presets,
          "absent.none-recorded",
        ),
      },
    ],
    archived:
      identity.kind === "archived"
        ? {
            successor: dashable(identity.successor, "absent.none-recorded"),
          }
        : null,
    provenance: bundle.rules.map((rule): ProvenanceRow => {
      const via = identity.imports
        .filter((imp) => imp.parts.some((p) => rule.scope.parts.includes(p)))
        .map((imp) => imp.rulebook);
      return {
        rule: rule.id,
        origin: rule.origin,
        via:
          rule.origin === "imported" && via.length > 0
            ? { kind: "value", value: via }
            : dash("absent.none-recorded"),
      };
    }),
  };
}
