// The changelog view's selectors over the fixture bundle, in node:
// releases newest first with their bump kind, one row per change with its
// consumer action, remedy and diff link, the filters, the counts against the
// changelog document the Go side writes, and, by absence, no field the list
// does not name.
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import {
  BundleDocument,
  ChangelogDocument,
  FieldListSchema,
  parse,
  recordReads,
  unlisted,
} from "@gragas/contract";
import { describe, expect, it } from "vitest";
import { documents, type Bundle } from "../../test/support.ts";
import { changelogView, newestFirst } from "./selectors.ts";

function fixture(): Bundle {
  const parsed = parse(BundleDocument, documents().get("bundle"));
  if (parsed.status !== "ok") throw new Error("the fixture must parse");
  return parsed.value;
}

const bundle = fixture();
const view = changelogView(bundle);

describe("the releases", () => {
  it("are the fixture's three, newest first, each with its bump kind and date", () => {
    expect(view.releases.map((r) => r.version)).toEqual([
      "1.2.0",
      "1.1.0",
      "1.0.0",
    ]);
    expect(view.releases.map((r) => r.bump)).toEqual([
      { kind: "value", value: "minor" },
      { kind: "value", value: "minor" },
      { kind: "value", value: "major" },
    ]);
    expect(view.releases[0]?.date).toEqual({
      kind: "value",
      value: "2026-11-07",
    });
  });

  it("order by number, not by text", () => {
    expect(["1.9.0", "1.10.0", "2.0.0", "1.2.3"].sort(newestFirst)).toEqual([
      "2.0.0",
      "1.10.0",
      "1.9.0",
      "1.2.3",
    ]);
  });

  it("show a dash with its reason when the bundle states no bump or date", () => {
    const bare = changelogView({
      ...bundle,
      changelog: bundle.changelog.map((e) => ({
        ...e,
        bump: null,
        date: null,
      })),
    });
    expect(bare.releases[0]?.bump).toEqual({
      kind: "dash",
      reason: "absent.not-in-bundle",
    });
    expect(bare.releases[0]?.date.kind).toBe("dash");
  });
});

describe("the changes", () => {
  const rows = view.releases.flatMap((r) => r.rows);

  it("are one row per changelog entry, none lost and none made", () => {
    expect(rows).toHaveLength(bundle.changelog.length);
    expect(view.changeCount).toBe(bundle.changelog.length);
    expect(view.shown).toBe(bundle.changelog.length);
  });

  it("carry the action and the remedy as the changelog states them", () => {
    const first = rows.find(
      (r) => r.entity === "rule:go.errors.1" && r.change === "added",
    );
    expect(first?.action).toEqual({
      kind: "given",
      text: "Wrap every error a service returns with what was being done.",
    });
    expect(first?.remedy).toEqual({
      kind: "value",
      value: "Wrap the error with fmt.Errorf and %w.",
    });
  });

  it("read a change with no action as none, and no remedy as a dash", () => {
    const layout = rows.find((r) => r.entity === "rule:go.layout.1");
    expect(layout?.action).toEqual({ kind: "none" });
    expect(layout?.remedy).toEqual({
      kind: "dash",
      reason: "absent.none-recorded",
    });
  });

  it("keep the diff link of a change that has one, and no other", () => {
    expect(rows.filter((r) => r.diff !== null).map((r) => r.diff)).toEqual([
      "https://example.com/fixture-base/compare/1.0.0...1.1.0",
    ]);
  });

  it("mark a replaced rule as leaving, still linked to its rule view", () => {
    const gone = rows.find((r) => r.entity === "rule:base.naming.2");
    expect(gone?.leaves).toBe(true);
    expect(gone?.ruleView).toBe(true);
  });

  it("link no rule view for a term or for a rule the bundle does not hold", () => {
    expect(rows.find((r) => r.entity === "term:gate")?.ruleView).toBe(false);
    const orphan = changelogView({ ...bundle, rules: [] });
    expect(orphan.releases.flatMap((r) => r.rows).some((r) => r.ruleView)).toBe(
      false,
    );
  });
});

describe("the filters", () => {
  it("keep one kind of change, and drop a release left with none", () => {
    const only = changelogView(bundle, { change: "replaced" });
    expect(only.releases.map((r) => r.version)).toEqual(["1.1.0"]);
    expect(only.shown).toBe(1);
    expect(only.changeCount).toBe(bundle.changelog.length);
  });

  it("keep the changes with no action, or those with one", () => {
    const none = changelogView(bundle, { action: "none" });
    const some = changelogView(bundle, { action: "some" });
    expect(none.shown + some.shown).toBe(bundle.changelog.length);
    expect(
      none.releases
        .flatMap((r) => r.rows)
        .every((r) => r.action.kind === "none"),
    ).toBe(true);
  });

  it("combine, and can leave nothing", () => {
    expect(changelogView(bundle, { change: "retired" }).releases).toEqual([]);
    expect(
      changelogView(bundle, { change: "added", action: "some" }).shown,
    ).toBe(1);
  });

  it("offer only the kinds the bundle holds", () => {
    expect(view.kinds).toEqual(["added", "changed", "replaced"]);
  });
});

describe("agreement with the changelog document", () => {
  const parsed = parse(ChangelogDocument, documents().get("changelog"));
  if (parsed.status !== "ok") throw new Error("the fixture must parse");
  const document = parsed.value;

  it("has the same releases, in the same order, with the same counts", () => {
    expect(view.releases.map((r) => [r.version, r.total])).toEqual(
      document.releases
        .map((r) => [r.version, r.entries.length] as const)
        .sort(([a], [b]) => newestFirst(a, b)),
    );
    expect(view.releaseCount).toBe(document.releases.length);
    expect(view.changeCount).toBe(
      document.releases.reduce((n, r) => n + r.entries.length, 0),
    );
  });

  it("states the same consumer action for each change", () => {
    const actions = new Map(
      document.releases.flatMap((r) =>
        r.entries.map((e) => [`${r.version} ${e.entity}`, e.consumer_action]),
      ),
    );
    for (const block of view.releases) {
      for (const row of block.rows) {
        expect(row.action.kind === "none" ? "" : row.action.text).toBe(
          actions.get(`${block.version} ${row.entity}`),
        );
      }
    }
  });
});

describe("the bundle's own words", () => {
  it("reads only the bundle, and only fields the list names", () => {
    const recorder = recordReads(bundle);
    changelogView(recorder.value, { change: "added", action: "some" });
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
    expect(unlisted(list, "changelog", "bundle", reads)).toEqual([]);
  });
});
