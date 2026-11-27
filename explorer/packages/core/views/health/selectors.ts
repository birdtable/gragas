// The health view's selectors: the seven numbers the release
// claims, in the order the spec names them, each with its unit and the key of
// the words that derive it; the sets (an overlay has two, S54); the proof pass
// rate's honest counts; and the changes of this release from the changelog.
// They read the one parsed bundle and take nothing else. A number is the one
// the release computed, never recomputed here; a value the bundle does not
// state is a dash with its reason (web.states.5). Health names no project and
// folds no signal in (S29).
import type { BundleDocument } from "@gragas/contract";
import type * as z from "zod";
import {
  dash,
  dashable,
  type Dashable,
  type MessageKey,
} from "../../messages/index.ts";

type Bundle = z.output<typeof BundleDocument>;

/** The seven names, in the order of S29. */
export const healthNames = [
  "rules-held",
  "entries-explained",
  "proof-pass-rate",
  "completeness",
  "review-currency",
  "attention-size",
  "days-to-active-median",
] as const;

export type HealthName = (typeof healthNames)[number];

/** A ratio is shown as a percentage, a count as itself, a median in days. */
export type Unit = "ratio" | "count" | "days";

const units: Readonly<Record<HealthName, Unit>> = {
  "rules-held": "ratio",
  "entries-explained": "ratio",
  "proof-pass-rate": "ratio",
  completeness: "ratio",
  "review-currency": "ratio",
  "attention-size": "count",
  "days-to-active-median": "days",
};

/** The message keys of a number's label and its derivation, from its name. */
export const labelKeys = {
  "rules-held": "health.label.rules-held",
  "entries-explained": "health.label.entries-explained",
  "proof-pass-rate": "health.label.proof-pass-rate",
  completeness: "health.label.completeness",
  "review-currency": "health.label.review-currency",
  "attention-size": "health.label.attention-size",
  "days-to-active-median": "health.label.days-to-active-median",
} as const satisfies Readonly<Record<HealthName, MessageKey>>;

export const derivationKeys = {
  "rules-held": "health.derivation.rules-held",
  "entries-explained": "health.derivation.entries-explained",
  "proof-pass-rate": "health.derivation.proof-pass-rate",
  completeness: "health.derivation.completeness",
  "review-currency": "health.derivation.review-currency",
  "attention-size": "health.derivation.attention-size",
  "days-to-active-median": "health.derivation.days-to-active-median",
} as const satisfies Readonly<Record<HealthName, MessageKey>>;

/** What the proof grid says about the pass rate: only a run that passed or failed counts. */
export interface ProofCounts {
  readonly ran: number;
  readonly passed: number;
  readonly notRun: number;
}

export interface HealthTile {
  readonly name: HealthName;
  readonly unit: Unit;
  readonly label: MessageKey;
  readonly derivation: MessageKey;
  /** The number the release computed, or a dash when the set lacks it. */
  readonly value: Dashable<number>;
  /** The change against the previous release: the bundle keeps only this release's numbers. */
  readonly change: Dashable<number>;
  /** For the proof pass rate alone: how many ran, of how many results. */
  readonly proof: ProofCounts | null;
}

export type SetRole = "own" | "resolved";

export interface HealthSet {
  readonly role: SetRole;
  /** Seven tiles, or a dash for the whole set when the bundle does not carry it. */
  readonly tiles: Dashable<readonly HealthTile[]>;
}

export interface ChangeCount {
  readonly change: Bundle["changelog"][number]["change"];
  readonly count: number;
}

export interface ChangeSummary {
  /** The release the changelog entries below are of. */
  readonly release: string;
  /** The release before it that the changelog names, if it names one. */
  readonly previous: Dashable<string>;
  readonly total: number;
  readonly byKind: readonly ChangeCount[];
}

export interface HealthView {
  readonly release: string;
  readonly kind: Bundle["identity"]["kind"];
  readonly sets: readonly HealthSet[];
  readonly changes: ChangeSummary;
}

const kinds = [
  "added",
  "changed",
  "weakened",
  "tightened",
  "retired",
  "restored",
  "replaced",
] as const;

function version(text: string): number[] {
  return text.split(".").map(Number);
}

function earlier(a: string, b: string): number {
  const x = version(a);
  const y = version(b);
  for (let i = 0; i < 3; i += 1) {
    const d = (x[i] ?? 0) - (y[i] ?? 0);
    if (d !== 0) return d;
  }
  return 0;
}

function proofCounts(bundle: Bundle): ProofCounts {
  const results = bundle.proof_grid.map((p) => p.result);
  const passed = results.filter((r) => r === "pass").length;
  const failed = results.filter((r) => r === "fail").length;
  return {
    ran: passed + failed,
    passed,
    notRun: results.filter((r) => r === "not run").length,
  };
}

function tilesOf(
  numbers: Bundle["health"]["numbers"],
  proof: ProofCounts,
): readonly HealthTile[] {
  return healthNames.map((name) => {
    const found = numbers.find((n) => n.name === name);
    return {
      name,
      unit: units[name],
      label: labelKeys[name],
      derivation: derivationKeys[name],
      value: dashable(found?.value, "absent.not-in-bundle"),
      change: dash("absent.contract-gap"),
      proof: name === "proof-pass-rate" ? proof : null,
    };
  });
}

function changesOf(bundle: Bundle): ChangeSummary {
  const release = bundle.release.version;
  const entries = bundle.changelog.filter((c) => c.release === release);
  const before = [
    ...new Set(
      bundle.changelog
        .map((c) => c.release)
        .filter((r) => earlier(r, release) < 0),
    ),
  ].sort(earlier);
  return {
    release,
    previous: dashable(before.at(-1), "absent.none-recorded"),
    total: entries.length,
    byKind: kinds
      .map((change) => ({
        change,
        count: entries.filter((e) => e.change === change).length,
      }))
      .filter((c) => c.count > 0),
  };
}

export function healthView(bundle: Bundle): HealthView {
  const proof = proofCounts(bundle);
  const held: HealthSet = {
    role: bundle.health.resolved ? "resolved" : "own",
    tiles: {
      kind: "value",
      value: tilesOf(bundle.health.numbers, proof),
    },
  };
  // An overlay publishes two labelled sets (S54); the contract carries one
  // set per bundle, so the other is a dash until the contract names it.
  const other: HealthSet[] =
    bundle.identity.kind === "overlay"
      ? [
          {
            role: held.role === "own" ? "resolved" : "own",
            tiles: dash("absent.contract-gap"),
          },
        ]
      : [];
  const sets = [held, ...other].sort(
    (a, b) => Number(b.role === "own") - Number(a.role === "own"),
  );
  return {
    release: bundle.release.version,
    kind: bundle.identity.kind,
    sets,
    changes: changesOf(bundle),
  };
}
