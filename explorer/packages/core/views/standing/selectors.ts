// The standing view's selectors: how many of the bundle's rules sit
// in each standing, overall and per part or kind, with the marks that sit beside
// a standing. A consumer sums what the bundle states, so every count here is a
// sum over `rules`; nothing is precomputed. Pure, so a node test judges it.
import type { BundleDocument } from "@gragas/contract";
import type * as z from "zod";
import { dash, type Dashable } from "../../messages/index.ts";

type Bundle = z.output<typeof BundleDocument>;
type Rule = Bundle["rules"][number];

export type Standing = Rule["standing"];
export type Mark = Rule["marks"][number];

export type By = "part" | "kind";

/** What the address carries: the grouping and the two filters. */
export interface StandingQuery {
  readonly by?: By | undefined;
  readonly part?: string | undefined;
  readonly kind?: string | undefined;
}

export interface Segment {
  readonly standing: Standing;
  readonly count: number;
}

/** The rules of one standing that carry one mark: a hollow, outlined or dotted segment. */
export interface MarkedSegment {
  readonly standing: Standing;
  readonly mark: Mark;
  readonly count: number;
}

export interface Bar {
  readonly total: number;
  /** One segment per standing, in the order of the tokens, zero counts kept. */
  readonly segments: readonly Segment[];
  readonly marked: readonly MarkedSegment[];
}

export interface Group extends Bar {
  /** The part or kind this bar is for; a dash for rules whose scope names no kind. */
  readonly key: Dashable<string>;
  readonly id: string;
}

export interface StandingView {
  /** The one bar of the chain: what the embedded rendering shows. */
  readonly chain: Bar;
  readonly by: By;
  readonly groups: readonly Group[];
  readonly parts: readonly string[];
  readonly kinds: readonly string[];
  readonly applied: readonly {
    readonly name: string;
    readonly value: string;
  }[];
}

export const standingOrder = [
  "proposed",
  "enforced",
  "reviewed",
  "planned",
  "advisory",
  "retired",
] as const satisfies readonly Standing[];

export const markOrder = [
  "trial",
  "blocked",
  "unproven",
] as const satisfies readonly Mark[];

/** A rule's part: the first segment of its id, which is how the rulebook names a part. */
export const partOf = (rule: Rule): string => rule.id.split(".")[0] ?? rule.id;

function barOf(rules: readonly Rule[]): Bar {
  const segments = standingOrder.map((standing) => ({
    standing,
    count: rules.filter((r) => r.standing === standing).length,
  }));
  const marked = standingOrder.flatMap((standing) =>
    markOrder.flatMap((mark) => {
      const count = rules.filter(
        (r) => r.standing === standing && r.marks.includes(mark),
      ).length;
      return count === 0 ? [] : [{ standing, mark, count }];
    }),
  );
  return { total: rules.length, segments, marked };
}

/** The standing view of `bundle` under the grouping and filters the address carries. */
export function standingView(
  bundle: Bundle,
  query: StandingQuery = {},
): StandingView {
  const by = query.by ?? "part";
  const parts = [...new Set(bundle.rules.map(partOf))].sort();
  const kinds = bundle.structure.kinds.map((k) => k.slug);
  const kept = bundle.rules.filter(
    (r) =>
      (query.part === undefined || partOf(r) === query.part) &&
      (query.kind === undefined || r.scope.kinds.includes(query.kind)),
  );
  const groups: Group[] =
    by === "part"
      ? parts.flatMap((part) => {
          const own = kept.filter((r) => partOf(r) === part);
          return own.length === 0
            ? []
            : [
                {
                  id: part,
                  key: { kind: "value" as const, value: part },
                  ...barOf(own),
                },
              ];
        })
      : [
          ...kinds.flatMap((kind) => {
            const own = kept.filter((r) => r.scope.kinds.includes(kind));
            return own.length === 0
              ? []
              : [
                  {
                    id: kind,
                    key: { kind: "value" as const, value: kind },
                    ...barOf(own),
                  },
                ];
          }),
          ...(() => {
            const none = kept.filter((r) => r.scope.kinds.length === 0);
            return none.length === 0
              ? []
              : [
                  {
                    id: "",
                    key: dash("absent.none-recorded"),
                    ...barOf(none),
                  },
                ];
          })(),
        ];
  return {
    chain: barOf(kept),
    by,
    groups,
    parts,
    kinds,
    applied: [
      ...(query.part === undefined
        ? []
        : [{ name: "part", value: query.part }]),
      ...(query.kind === undefined
        ? []
        : [{ name: "kind", value: query.kind }]),
    ],
  };
}
