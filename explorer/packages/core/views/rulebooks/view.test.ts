// The rulebooks view's selectors over the fixture bundle, in node:
// the chain with its pins, the imports by part, the counts
// of the rule map, governance and licences per kind, an archived rulebook, the
// provenance of each rule's layer; and, by absence, no list of projects that pin
// the chain and no withdrawn list.
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
import { rulebooksView } from "./selectors.ts";

function fixture(): Bundle {
  const parsed = parse(BundleDocument, documents().get("bundle"));
  if (parsed.status !== "ok") throw new Error("the fixture must parse");
  return parsed.value;
}

const bundle = fixture();
const view = rulebooksView(bundle);

describe("the fixture overlay", () => {
  it("shows the parent, the overlay and the import with their pins", () => {
    expect(view.chain.map((b) => [b.role, b.name, b.version])).toEqual([
      ["parent", "fixture-parent", "0.9.0"],
      ["this", "fixture-base", "1.2.0"],
      ["import", "fixture-other", "0.3.0"],
    ]);
    expect(view.kind).toBe("overlay");
  });

  it("lists the import with the parts it took", () => {
    expect(view.imports).toEqual([
      {
        rulebook: "fixture-other",
        version: "0.3.0",
        parts: ["ui"],
      },
    ]);
    expect(view.chain[2]?.parts).toEqual(["ui"]);
  });

  it("counts the rule map as kept, tighter, weaker, gone and new", () => {
    expect(view.ruleMaps).toEqual([
      {
        rulebook: "fixture-other",
        kept: 1,
        tighter: 1,
        weaker: 1,
        gone: 1,
        new: 0,
        arrived: 3,
        left: 4,
      },
    ]);
  });

  it("states governance and the licence of each kind", () => {
    expect(view.governance).toEqual({
      contribution: "proposals",
      decisionRequired: true,
      trialReleases: 2,
      reviewDays: 180,
    });
    expect(view.licences).toEqual([
      { kind: "rules", licence: { kind: "value", value: "CC-BY-4.0" } },
      { kind: "checker-code", licence: { kind: "value", value: "Apache-2.0" } },
      { kind: "presets", licence: { kind: "value", value: "MIT" } },
    ]);
  });

  it("names where each rule's layer came from", () => {
    const origin = new Map(view.provenance.map((p) => [p.rule, p.origin]));
    expect(origin.get("base.errors.1")).toBe("imported");
    expect(origin.get("go.errors.1")).toBe("authored");
    expect(origin.get("go.errors.2")).toBe("migrated");
    expect(view.provenance).toHaveLength(bundle.rules.length);
  });
});

describe("a rulebook with nothing to chain", () => {
  const alone: Bundle = {
    ...bundle,
    identity: { ...bundle.identity, extends: null, imports: [] },
    rule_map: [],
  };

  it("shows itself alone, with no parent and no import", () => {
    const view = rulebooksView(alone);
    expect(view.chain).toEqual([
      {
        role: "this",
        name: "fixture-base",
        version: "1.2.0",
        parts: [],
      },
    ]);
    expect(view.imports).toEqual([]);
    expect(view.ruleMaps).toEqual([]);
  });
});

describe("an archived rulebook", () => {
  // The successor is a field of the identity (identity.successor).
  it("shows the successor its identity names, and a dash when it names none", () => {
    const archived: Bundle = {
      ...bundle,
      identity: { ...bundle.identity, kind: "archived" },
    };
    expect(rulebooksView(archived).archived).toEqual({
      successor: { kind: "value", value: "fixture-other" },
    });
    const orphan: Bundle = {
      ...bundle,
      identity: { ...bundle.identity, kind: "archived", successor: null },
    };
    expect(rulebooksView(orphan).archived).toEqual({
      successor: { kind: "dash", reason: "absent.none-recorded" },
    });
    expect(view.archived).toBeNull();
  });
});

describe("what the view does not show", () => {
  it("lists no project that pins the chain and no withdrawn list: asserted by absence", () => {
    const names = JSON.stringify(view).toLowerCase();
    for (const word of [
      "project",
      "pinned",
      "pinnedby",
      "withdrawn",
      "revoked",
    ]) {
      expect(names).not.toContain(word);
    }
    expect(Object.keys(view).sort()).toEqual([
      "archived",
      "chain",
      "governance",
      "imports",
      "kind",
      "licences",
      "name",
      "provenance",
      "ruleMaps",
      "version",
    ]);
  });

  it("reads only the bundle, and only fields the list names", () => {
    const recorder = recordReads(bundle);
    rulebooksView(recorder.value);
    const reads = recorder.reads();
    expect(reads.length).toBeGreaterThan(10);
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
    expect(unlisted(list, "rulebooks", "bundle", reads)).toEqual([]);
  });
});
