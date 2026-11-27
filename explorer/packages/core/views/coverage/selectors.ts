// The coverage view's selectors: rules to entries and entries to
// rules, by part and by tool, under the standing the reader chose. They read
// the one parsed bundle: a rule is held when the bundle's rules_to_bindings
// names it, an entry is traced when bindings_to_rules names it. Nothing is
// recomputed differently from the contract (the derivation is the contract's);
// the tool's and the part's rows only group what the bundle already states.
// A value the bundle does not state is a dash with its reason (web.states.5).
import type { BundleDocument } from "@gragas/contract";
import type * as z from "zod";
import { dash, type Dashable } from "../../messages/index.ts";
import type { Standing } from "../../parts/index.ts";

type Bundle = z.output<typeof BundleDocument>;

/** The six standings, in the order of the legend. */
export const standingOrder = [
  "enforced",
  "reviewed",
  "planned",
  "advisory",
  "proposed",
  "retired",
] as const satisfies readonly Standing[];

export type Chosen = Standing | null;

/** n of m: the numerator the bundle's lists give over the set they count. */
export interface Ratio {
  readonly n: number;
  readonly of: number;
}

export interface PartRow {
  readonly slug: string;
  readonly name: string;
  readonly layer: string;
  /** Live rules held, over live rules; a dash when the part has none. */
  readonly rules: Dashable<Ratio>;
  /** Entries traced to a rule, over the part's entries; a dash when it has none. */
  readonly entries: Dashable<Ratio>;
  readonly segments: readonly { standing: Standing; count: number }[];
}

export interface ToolRow {
  readonly tool: string;
  readonly entries: Ratio;
}

export interface UpstreamSet {
  readonly rulebook: string;
  readonly version: string;
  /** The rules the upstream set supplies: one covered unit, expanded to these. */
  readonly rules: readonly { id: string; held: boolean }[];
}

export interface CoverageView {
  readonly release: string;
  readonly chosen: Chosen;
  readonly held: Ratio;
  readonly explained: Ratio;
  readonly parts: readonly PartRow[];
  readonly tools: readonly ToolRow[];
  readonly upstream: Dashable<UpstreamSet>;
  readonly ruleCount: number;
}

const live = (r: Bundle["rules"][number]): boolean =>
  r.status !== "retired" && r.status !== "proposed";

const partOf = (id: string): string => id.split(".")[0] ?? "";

const entryOf = (b: Bundle["bindings"][number]): string => `${b.tool}/${b.key}`;

function ratio(n: number, of: number): Dashable<Ratio> {
  return of === 0
    ? dash("absent.none-in-part")
    : { kind: "value", value: { n, of } };
}

export function coverageView(bundle: Bundle, chosen: Chosen): CoverageView {
  const held = new Set(bundle.coverage.rules_to_bindings);
  const traced = new Set(bundle.coverage.bindings_to_rules);
  const standingOf = new Map(bundle.rules.map((r) => [r.id, r.standing]));
  const wanted = (ruleId: string): boolean =>
    chosen === null || standingOf.get(ruleId) === chosen;

  const liveRules = bundle.rules.filter((r) => live(r) && wanted(r.id));
  const entries = new Map<string, { tool: string; rules: Set<string> }>();
  for (const b of bundle.bindings) {
    if (!wanted(b.rule)) continue;
    const key = entryOf(b);
    const found = entries.get(key) ?? {
      tool: b.tool,
      rules: new Set<string>(),
    };
    found.rules.add(b.rule);
    entries.set(key, found);
  }

  const parts: PartRow[] = bundle.structure.parts.map((part) => {
    const mine = liveRules.filter((r) => partOf(r.id) === part.slug);
    const theirs = [...entries].filter(([, e]) =>
      [...e.rules].some((id) => partOf(id) === part.slug),
    );
    return {
      slug: part.slug,
      name: part.name,
      layer:
        bundle.structure.layers.find((l) => l.slug === part.layer)?.name ??
        part.layer,
      rules: ratio(mine.filter((r) => held.has(r.id)).length, mine.length),
      entries: ratio(
        theirs.filter(([key]) => traced.has(key)).length,
        theirs.length,
      ),
      segments: standingOrder.map((standing) => ({
        standing,
        count: bundle.rules.filter(
          (r) => partOf(r.id) === part.slug && r.standing === standing,
        ).length,
      })),
    };
  });

  const toolNames = [
    ...new Set([...entries.values()].map((e) => e.tool)),
  ].sort();
  const tools = toolNames.map((tool) => {
    const mine = [...entries].filter(([, e]) => e.tool === tool);
    return {
      tool,
      entries: {
        n: mine.filter(([key]) => traced.has(key)).length,
        of: mine.length,
      },
    };
  });

  const extended = bundle.identity.extends;
  const imported = bundle.rules.filter(
    (r) => r.origin === "imported" && wanted(r.id),
  );
  const upstream: Dashable<UpstreamSet> =
    extended === null || extended === undefined || imported.length === 0
      ? dash("absent.none-recorded")
      : {
          kind: "value",
          value: {
            rulebook: extended.rulebook,
            version: extended.version,
            rules: imported.map((r) => ({ id: r.id, held: held.has(r.id) })),
          },
        };

  return {
    release: bundle.release.version,
    chosen,
    held: {
      n: liveRules.filter((r) => held.has(r.id)).length,
      of: liveRules.length,
    },
    explained: {
      n: [...entries.keys()].filter((k) => traced.has(k)).length,
      of: entries.size,
    },
    parts,
    tools,
    upstream,
    ruleCount: bundle.rules.length,
  };
}
