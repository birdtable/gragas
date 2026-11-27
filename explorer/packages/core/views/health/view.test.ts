// The health view's selectors over the fixture bundle, in node:
// the seven numbers as the release computed them, in S29's order; the two sets
// of an overlay with the missing one a dash that cites the contract gap; the
// proof pass rate's honest counts; the changes of the release from the
// changelog; and, by absence, no number that is not in the bundle.
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import {
  BundleDocument,
  FieldListSchema,
  parse,
  recordReads,
  unlisted,
} from "@gragas/contract";
import { describe, expect, it } from "vitest";
import { documents, type Bundle } from "../../test/support.ts";
import { healthNames, healthView } from "./selectors.ts";

function fixture(): Bundle {
  const parsed = parse(BundleDocument, documents().get("bundle"));
  if (parsed.status !== "ok") throw new Error("the fixture must parse");
  return parsed.value;
}

const bundle = fixture();
const view = healthView(bundle);

describe("the fixture overlay", () => {
  it("has the own set with the seven numbers in the order of S29", () => {
    const own = view.sets.find((s) => s.role === "own");
    expect(own?.tiles.kind).toBe("value");
    const tiles = own?.tiles.kind === "value" ? own.tiles.value : [];
    expect(tiles.map((t) => t.name)).toEqual([...healthNames]);
    expect(tiles.map((t) => t.value)).toEqual(
      [0.8462, 1, 0.75, 0.2727, 1, 11, 1].map((value) => ({
        kind: "value",
        value,
      })),
    );
  });

  it("agrees with the numbers the release claims", () => {
    const own = view.sets.find((s) => s.role === "own");
    const tiles = own?.tiles.kind === "value" ? own.tiles.value : [];
    const claimed = bundle.release.health?.numbers.map((n) => n.value);
    expect(
      tiles.map((t) => (t.value.kind === "value" ? t.value.value : null)),
    ).toEqual(claimed);
  });

  it("labels two sets, the second a dash that cites the contract gap", () => {
    expect(view.sets.map((s) => s.role)).toEqual(["own", "resolved"]);
    expect(view.sets[1]?.tiles).toEqual({
      kind: "dash",
      reason: "absent.contract-gap",
    });
  });

  it("states the proof pass rate with how many ran and how many did not", () => {
    const own = view.sets[0]?.tiles;
    const rate =
      own?.kind === "value"
        ? own.value.find((t) => t.name === "proof-pass-rate")
        : undefined;
    expect(rate?.proof).toEqual({ ran: 4, passed: 3, notRun: 13 });
  });

  it("shows no change against the previous release as a dash", () => {
    const own = view.sets[0]?.tiles;
    const changes = own?.kind === "value" ? own.value.map((t) => t.change) : [];
    expect(changes).toHaveLength(7);
    for (const c of changes) {
      expect(c).toEqual({ kind: "dash", reason: "absent.contract-gap" });
    }
  });

  it("counts the changes of this release and names the one before it", () => {
    expect(view.changes.release).toBe("1.2.0");
    expect(view.changes.previous).toEqual({ kind: "value", value: "1.1.0" });
    const entries = bundle.changelog.filter((c) => c.release === "1.2.0");
    expect(view.changes.total).toBe(entries.length);
    expect(view.changes.byKind.reduce((n, c) => n + c.count, 0)).toBe(
      entries.length,
    );
  });
});

describe("a base rulebook", () => {
  it("has one set and no second set to dash", () => {
    const base = healthView({
      ...bundle,
      identity: {
        ...bundle.identity,
        kind: "base",
        extends: null,
        imports: [],
      },
    });
    expect(base.sets.map((s) => s.role)).toEqual(["own"]);
  });
});

describe("the bundle's own words", () => {
  it("shows no number the bundle does not hold", () => {
    const held = new Set(bundle.health.numbers.map((n) => n.value));
    const shown = view.sets
      .flatMap((s) => (s.tiles.kind === "value" ? s.tiles.value : []))
      .flatMap((t) => (t.value.kind === "value" ? [t.value.value] : []));
    expect(shown.filter((v) => !held.has(v))).toEqual([]);
  });

  it("reads only the bundle, and only fields the list names", () => {
    const recorder = recordReads(bundle);
    healthView(recorder.value);
    const reads = recorder.reads();
    expect(reads.length).toBeGreaterThan(3);
    const list = FieldListSchema.parse(
      JSON.parse(
        readFileSync(
          fileURLToPath(
            new URL("../../../../contract/fields.json", import.meta.url),
          ),
          "utf8",
        ),
      ),
    );
    expect(unlisted(list, "health", "bundle", reads)).toEqual([]);
  });
});
