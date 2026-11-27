// The tools view's selectors: each tool of the bundle with its
// tested and adapter versions, its entries and the rules that hold each, the
// entries nothing holds, and a standing bar per tool. Pure, so a node test judges
// it; the page draws what comes back and recomputes nothing (web.structure.4).
//
// Tool names come from the bundle (arch.ports.2). An entry is a tool's config
// line: its tool, its key and the kind of target it sits on. Entries a rule holds
// at rung 0 are an upstream preset adopted whole: they fold under that rule
// (spec S47), and the page lets a reader expand each fold.
import type { BundleDocument } from "@gragas/contract";
import type * as z from "zod";
import { dash, dashable, type Dashable } from "../../messages/index.ts";

type Bundle = z.output<typeof BundleDocument>;
type Binding = Bundle["bindings"][number];

export type EntryKind = Binding["target_type"];
export type Standing = Bundle["rules"][number]["standing"];

export type Grouping = "tool" | "kind";

/** What the address carries: the filters, the grouping and the open tool. */
export interface ToolsQuery {
  readonly tool?: string | undefined;
  readonly kind?: EntryKind | undefined;
  readonly unmapped?: boolean | undefined;
  readonly group?: Grouping | undefined;
  readonly open?: string | undefined;
}

export interface Holder {
  readonly rule: string;
  readonly rung: number;
  /** What the binding sets, as text; a dash when it sets none. */
  readonly value: Dashable<string>;
}

export interface EntryRow {
  /** `tool/key`, the entry's id in the bundle's coverage and in a rule's own list. */
  readonly id: string;
  readonly tool: string;
  readonly key: string;
  readonly kind: EntryKind;
  readonly holders: readonly Holder[];
}

/** The entries one rule holds at rung 0: a preset adopted whole, one fold. */
export interface Fold {
  readonly rule: string;
  readonly entries: readonly EntryRow[];
}

export interface Group {
  /** The tool or the kind this group is for. */
  readonly key: string;
  readonly entries: readonly EntryRow[];
  readonly folds: readonly Fold[];
  /** Entries a preset of the tool carries that no rule holds, when the tool has a preset. */
  readonly unmapped: number;
}

export interface Segment {
  readonly standing: Standing;
  readonly count: number;
}

export interface ToolRow {
  readonly tool: string;
  /** The version the release was proven on. */
  readonly version: Dashable<string>;
  /** The adapter's version the bundle used. */
  readonly adapter: Dashable<string>;
  readonly entries: number;
  readonly rules: number;
  readonly unmapped: number;
  readonly standings: readonly Segment[];
}

export interface Counts {
  readonly tools: number;
  readonly entries: number;
  readonly rules: number;
  readonly unmapped: number;
}

export interface ToolsView {
  /** Every tool, whatever the filters say: they are the side panel and the filter's choices. */
  readonly tools: readonly ToolRow[];
  readonly kinds: readonly EntryKind[];
  readonly grouping: Grouping;
  readonly groups: readonly Group[];
  /** The counts of what the filters leave. */
  readonly counts: Counts;
  readonly applied: readonly {
    readonly name: string;
    readonly value: string;
  }[];
  /** The open tool's row, when the address names one the bundle holds. */
  readonly open: ToolRow | null;
  /** The names of unmapped entries: contract v1 carries a preset's entry count and no list of them, a gap in the contract, so the view says so. */
  readonly unmappedNames: Dashable<never>;
}

const standingOrder = [
  "enforced",
  "reviewed",
  "planned",
  "advisory",
  "proposed",
  "retired",
] as const satisfies readonly Standing[];

function text(value: unknown): Dashable<string> {
  if (value === null || value === undefined || value === "") {
    return dash("absent.none-recorded");
  }
  if (
    typeof value === "string" ||
    typeof value === "number" ||
    typeof value === "boolean"
  ) {
    return { kind: "value", value: String(value) };
  }
  // A list or a record is the rule view's to show; here it is named, not unpacked.
  return dash("absent.structured");
}

function entriesOf(bundle: Bundle): EntryRow[] {
  const byId = new Map<string, EntryRow & { holders: Holder[] }>();
  for (const b of bundle.bindings) {
    const id = `${b.tool}/${b.key}`;
    const held = byId.get(id) ?? {
      id,
      tool: b.tool,
      key: b.key,
      kind: b.target_type,
      holders: [],
    };
    held.holders.push({ rule: b.rule, rung: b.rung, value: text(b.value) });
    byId.set(id, held);
  }
  return [...byId.values()];
}

const kindsOf = (entries: readonly EntryRow[]): EntryKind[] =>
  [...new Set(entries.map((e) => e.kind))].sort();

function toolRows(bundle: Bundle, entries: readonly EntryRow[]): ToolRow[] {
  const standingOf = new Map(bundle.rules.map((r) => [r.id, r.standing]));
  const names = new Set<string>([
    ...bundle.tested_set.tools.map((t) => t.tool),
    ...bundle.presets.map((p) => p.tool),
    ...bundle.adapters_used.map((a) => a.tool),
    ...entries.map((e) => e.tool),
  ]);
  return [...names].sort().map((tool) => {
    const own = entries.filter((e) => e.tool === tool);
    const rules = new Set(own.flatMap((e) => e.holders.map((h) => h.rule)));
    const counts = new Map<Standing, number>();
    for (const rule of rules) {
      const standing = standingOf.get(rule);
      if (standing !== undefined) {
        counts.set(standing, (counts.get(standing) ?? 0) + 1);
      }
    }
    const preset = bundle.presets
      .filter((p) => p.tool === tool)
      .reduce((sum, p) => sum + p.entries, 0);
    return {
      tool,
      version: dashable(
        bundle.tested_set.tools.find((t) => t.tool === tool)?.version,
        "absent.not-in-bundle",
      ),
      adapter: dashable(
        bundle.adapters_used.find((a) => a.tool === tool)?.adapter_version,
        "absent.not-in-bundle",
      ),
      entries: own.length,
      rules: rules.size,
      unmapped: Math.max(0, preset - own.length),
      standings: standingOrder.flatMap((standing) => {
        const count = counts.get(standing) ?? 0;
        return count === 0 ? [] : [{ standing, count }];
      }),
    };
  });
}

function folded(entry: EntryRow): boolean {
  return entry.holders.every((h) => h.rung === 0);
}

function foldsOf(entries: readonly EntryRow[]): Fold[] {
  const byRule = new Map<string, EntryRow[]>();
  for (const entry of entries.filter(folded)) {
    for (const rule of new Set(entry.holders.map((h) => h.rule))) {
      byRule.set(rule, [...(byRule.get(rule) ?? []), entry]);
    }
  }
  return [...byRule].map(([rule, held]) => ({ rule, entries: held }));
}

/** The tools view of `bundle` under the filters and grouping the address carries. */
export function toolsView(bundle: Bundle, query: ToolsQuery = {}): ToolsView {
  const all = entriesOf(bundle);
  const tools = toolRows(bundle, all);
  const grouping = query.group ?? "tool";
  const only = query.unmapped === true;
  const inTool = (tool: string): boolean =>
    query.tool === undefined || tool === query.tool;
  // An unmapped entry has no kind the bundle states, so a kind filter leaves none.
  const unmappedOf = (tool: ToolRow): number =>
    query.kind === undefined ? tool.unmapped : 0;
  const shown = only
    ? []
    : all.filter(
        (e) =>
          inTool(e.tool) && (query.kind === undefined || e.kind === query.kind),
      );
  const keys =
    grouping === "tool"
      ? tools
          .filter((t) => inTool(t.tool) && (!only || unmappedOf(t) > 0))
          .map((t) => t.tool)
      : kindsOf(shown);
  const groups = keys.map((key): Group => {
    const own = shown.filter(
      (e) => (grouping === "tool" ? e.tool : e.kind) === key,
    );
    const tool = tools.find((t) => t.tool === key);
    return {
      key,
      entries: own.filter((e) => !folded(e)),
      folds: foldsOf(own),
      unmapped:
        grouping === "tool" && tool !== undefined ? unmappedOf(tool) : 0,
    };
  });
  const rules = new Set(shown.flatMap((e) => e.holders.map((h) => h.rule)));
  const present = new Set([
    ...shown.map((e) => e.tool),
    ...(grouping === "tool" ? keys : []),
  ]);
  const applied = [
    ...(query.tool === undefined ? [] : [{ name: "tool", value: query.tool }]),
    ...(query.kind === undefined ? [] : [{ name: "kind", value: query.kind }]),
    ...(only ? [{ name: "unmapped", value: "yes" }] : []),
  ];
  return {
    tools,
    kinds: kindsOf(all),
    grouping,
    groups,
    counts: {
      tools: present.size,
      entries: shown.length,
      rules: rules.size,
      unmapped: groups.reduce((sum, g) => sum + g.unmapped, 0),
    },
    applied,
    open: tools.find((t) => t.tool === query.open) ?? null,
    unmappedNames: dash("absent.not-in-bundle"),
  };
}
