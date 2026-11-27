// The changelog view's selectors: the bundle's changelog grouped by
// release, newest first, each release with its bump kind and date, and each
// change with its consumer action, its remedy and its diff link. They read the
// one parsed bundle and nothing else (web.structure.5). The view cannot say
// which action applies to a given project: upgrade works that out from the
// lock, so an action is shown as the changelog states it. A change whose action
// the bundle leaves empty is "none", said as such; a value the bundle does not
// state is a dash with its reason (web.states.5).
import type { BundleDocument } from "@gragas/contract";
import type * as z from "zod";
import { dashable, type Dashable } from "../../messages/index.ts";

type Bundle = z.output<typeof BundleDocument>;

/** The kinds of change the changelog records, in the contract's order. */
export const changeKinds = [
  "added",
  "changed",
  "weakened",
  "tightened",
  "retired",
  "restored",
  "replaced",
] as const;
export type ChangeKind = (typeof changeKinds)[number];

export type BumpKind = "major" | "minor" | "patch";

/** What the consumer does about a change: nothing, or the sentence the changelog gives. */
export type ActionFilter = "some" | "none";

export interface Filters {
  readonly change?: ChangeKind | undefined;
  readonly action?: ActionFilter | undefined;
}

export type EntityKind = "rule" | "term" | "checker-code" | "other";

export interface ChangeRow {
  /** The entity as the bundle names it: `rule:go.errors.1`. */
  readonly entity: string;
  readonly entityKind: EntityKind;
  /** The part of the entity after its kind: `go.errors.1`. */
  readonly id: string;
  readonly change: ChangeKind;
  readonly sentence: Dashable<string>;
  /** The consumer action, or "none" when the changelog states none. */
  readonly action:
    | { readonly kind: "none" }
    | { readonly kind: "given"; readonly text: string };
  readonly remedy: Dashable<string>;
  readonly diff: string | null;
  /** The entity is a rule the bundle holds, so its rule view exists. */
  readonly ruleView: boolean;
  /** The change retires or replaces the entity. */
  readonly leaves: boolean;
}

export interface ReleaseBlock {
  readonly version: string;
  readonly bump: Dashable<BumpKind>;
  readonly date: Dashable<string>;
  /** Every change of the release, before the filters. */
  readonly total: number;
  /** The changes that pass the filters. */
  readonly rows: readonly ChangeRow[];
}

export interface ChangelogView {
  /** Newest release first; a release with no matching change is left out. */
  readonly releases: readonly ReleaseBlock[];
  /** Every release of the changelog, with the filters off. */
  readonly releaseCount: number;
  readonly changeCount: number;
  readonly shown: number;
  /** The kinds of change present in the bundle, in the contract's order. */
  readonly kinds: readonly ChangeKind[];
}

function parts(version: string): readonly number[] {
  return version.split(".").map((p) => Number.parseInt(p, 10));
}

/** Semver order, newest first. */
export function newestFirst(a: string, b: string): number {
  const x = parts(a);
  const y = parts(b);
  for (let i = 0; i < 3; i += 1) {
    const d = (y[i] ?? 0) - (x[i] ?? 0);
    if (d !== 0) return d;
  }
  return 0;
}

function entityKindOf(kind: string): EntityKind {
  if (kind === "rule" || kind === "term" || kind === "checker-code")
    return kind;
  return "other";
}

function rowOf(
  entry: Bundle["changelog"][number],
  rules: ReadonlySet<string>,
): ChangeRow {
  const colon = entry.entity.indexOf(":");
  const kind = colon < 0 ? "" : entry.entity.slice(0, colon);
  const id = colon < 0 ? entry.entity : entry.entity.slice(colon + 1);
  const entityKind = entityKindOf(kind);
  return {
    entity: entry.entity,
    entityKind,
    id,
    change: entry.change,
    sentence: dashable(
      entry.sentence === "" ? null : entry.sentence,
      "absent.none-recorded",
    ),
    action:
      entry.consumer_action === ""
        ? { kind: "none" }
        : { kind: "given", text: entry.consumer_action },
    remedy: dashable(
      entry.remedy === "" ? null : entry.remedy,
      "absent.none-recorded",
    ),
    diff: entry.diff,
    ruleView: entityKind === "rule" && rules.has(id),
    leaves: entry.change === "retired" || entry.change === "replaced",
  };
}

function passes(row: ChangeRow, filters: Filters): boolean {
  if (filters.change !== undefined && row.change !== filters.change) {
    return false;
  }
  if (filters.action === "none" && row.action.kind !== "none") return false;
  if (filters.action === "some" && row.action.kind === "none") return false;
  return true;
}

/** The changelog of the bundle, grouped by release and filtered. */
export function changelogView(
  bundle: Bundle,
  filters: Filters = {},
): ChangelogView {
  const rules = new Set(bundle.rules.map((r) => r.id));
  const byRelease = new Map<string, Bundle["changelog"]>();
  for (const entry of bundle.changelog) {
    byRelease.set(entry.release, [
      ...(byRelease.get(entry.release) ?? []),
      entry,
    ]);
  }
  const blocks = [...byRelease.entries()]
    .sort(([a], [b]) => newestFirst(a, b))
    .map(([version, entries]): ReleaseBlock => {
      const bump = entries.find((e) => e.bump !== null)?.bump ?? null;
      const date = entries.find((e) => e.date !== null)?.date ?? null;
      return {
        version,
        bump: dashable(bump, "absent.not-in-bundle"),
        date: dashable(date, "absent.not-in-bundle"),
        total: entries.length,
        rows: entries
          .map((e) => rowOf(e, rules))
          .filter((row) => passes(row, filters)),
      };
    });
  const present = new Set(bundle.changelog.map((e) => e.change));
  return {
    releases: blocks.filter((b) => b.rows.length > 0),
    releaseCount: blocks.length,
    changeCount: bundle.changelog.length,
    shown: blocks.reduce((n, b) => n + b.rows.length, 0),
    kinds: changeKinds.filter((k) => present.has(k)),
  };
}
