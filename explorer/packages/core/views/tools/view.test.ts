// The tools view's selectors over the fixture bundle, in node: the
// tools with their versions, the entries and the rules that hold them, the
// unmapped count, the filters, both groupings and the rung-0 folds.
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
import { toolsView } from "./selectors.ts";

function fixture(): Bundle {
  const parsed = parse(BundleDocument, documents().get("bundle"));
  if (parsed.status !== "ok") throw new Error("the fixture must parse");
  return parsed.value;
}

const bundle = fixture();

/** The fixture with the first preset claiming more entries than any rule holds. */
function withUnmapped(extra: number): Bundle {
  return {
    ...bundle,
    presets: bundle.presets.map((p, i) =>
      i === 0 ? { ...p, entries: p.entries + extra } : p,
    ),
  };
}

describe("the tools of the fixture bundle", () => {
  const view = toolsView(bundle);

  it("lists every tool the bundle names, with its tested and adapter versions", () => {
    const golangci = view.tools.find((t) => t.tool === "golangci-lint");
    expect(golangci?.version).toEqual({ kind: "value", value: "2.1.0" });
    expect(golangci?.adapter).toEqual({ kind: "value", value: "1.0.0" });
    expect(view.tools.map((t) => t.tool)).toEqual(
      [...new Set(view.tools.map((t) => t.tool))].sort(),
    );
  });

  it("shows a version nobody tested as a dash that says why", () => {
    const sqlfluff = view.tools.find((t) => t.tool === "sqlfluff");
    expect(sqlfluff?.version).toEqual({
      kind: "dash",
      reason: "absent.not-in-bundle",
    });
    expect(sqlfluff?.adapter).toEqual({
      kind: "dash",
      reason: "absent.not-in-bundle",
    });
  });

  it("gives each entry the rules that hold it, with their rungs", () => {
    const group = view.groups.find((g) => g.key === "golangci-lint");
    const wrapcheck = group?.entries.find(
      (e) => e.id === "golangci-lint/wrapcheck",
    );
    expect(wrapcheck?.holders.map((h) => [h.rule, h.rung])).toEqual([
      ["go.errors.1", 1],
      ["go.layout.2", 2],
    ]);
  });

  it("draws a tool's standing bar from the rules that hold its entries", () => {
    const stylelint = view.tools.find((t) => t.tool === "stylelint");
    expect(stylelint?.standings).toEqual([{ standing: "enforced", count: 1 }]);
    const golangci = view.tools.find((t) => t.tool === "golangci-lint");
    const total = golangci?.standings.reduce((sum, s) => sum + s.count, 0);
    expect(total).toBe(golangci?.rules);
  });

  it("folds the entries a rule holds at rung 0 under that rule", () => {
    const group = view.groups.find((g) => g.key === "golangci-lint");
    expect(group?.folds.map((f) => f.rule)).toEqual([
      "base.errors.1",
      "go.errors.1",
    ]);
    expect(group?.entries.map((e) => e.key)).not.toContain("errorlint");
    // errorlint is held at rung 0 by two rules, so it sits in both folds.
    expect(group?.folds.flatMap((f) => f.entries.map((e) => e.key))).toEqual([
      "errorlint",
      "errorlint",
    ]);
  });

  it("counts what the view shows", () => {
    expect(view.counts.tools).toBe(view.tools.length);
    expect(view.counts.entries).toBe(
      new Set(bundle.bindings.map((b) => `${b.tool}/${b.key}`)).size,
    );
    expect(view.counts.rules).toBe(
      new Set(bundle.bindings.map((b) => b.rule)).size,
    );
  });

  it("states a rung-6 review question as the entry's value", () => {
    const review = view.groups
      .flatMap((g) => g.entries)
      .find((e) => e.tool === "review");
    expect(review?.holders[0]?.rung).toBe(6);
    expect(review?.holders[0]?.value.kind).toBe("value");
  });
});

describe("an entry no rule holds", () => {
  it("is counted as unmapped when a preset carries more entries than the rules hold", () => {
    const view = toolsView(withUnmapped(2));
    expect(view.counts.unmapped).toBe(2);
    expect(view.tools.find((t) => t.tool === "stylelint")?.unmapped).toBe(2);
  });

  it("is none when every entry of the preset is held", () => {
    expect(toolsView(bundle).counts.unmapped).toBe(0);
  });

  it("shows its names as a dash, since the contract lists none", () => {
    expect(toolsView(withUnmapped(1)).unmappedNames).toEqual({
      kind: "dash",
      reason: "absent.not-in-bundle",
    });
  });

  it("is the only thing the unmapped filter leaves", () => {
    const view = toolsView(withUnmapped(1), { unmapped: true });
    expect(
      view.groups.map((g) => [g.key, g.entries.length, g.unmapped]),
    ).toEqual([["stylelint", 0, 1]]);
    expect(view.counts).toMatchObject({ tools: 1, entries: 0, unmapped: 1 });
  });
});

describe("the filters and the grouping", () => {
  it("changes the counts when one tool is chosen, and says what is applied", () => {
    const all = toolsView(bundle);
    const one = toolsView(bundle, { tool: "stylelint" });
    expect(one.counts.entries).toBeLessThan(all.counts.entries);
    expect(one.counts).toMatchObject({ tools: 1, entries: 1, rules: 1 });
    expect(one.applied).toEqual([{ name: "tool", value: "stylelint" }]);
    expect(one.groups.map((g) => g.key)).toEqual(["stylelint"]);
  });

  it("keeps every tool in the side panel's list whatever the filter", () => {
    expect(toolsView(bundle, { tool: "stylelint" }).tools).toEqual(
      toolsView(bundle).tools,
    );
  });

  it("groups by kind of target, and the groups hold the same entries", () => {
    const byKind = toolsView(bundle, { group: "kind" });
    const byTool = toolsView(bundle);
    expect(byKind.grouping).toBe("kind");
    expect(byKind.groups.map((g) => g.key)).toEqual(byKind.kinds);
    const ids = (v: typeof byKind): string[] =>
      v.groups
        .flatMap((g) => [...g.entries, ...g.folds.flatMap((f) => f.entries)])
        .map((e) => e.id)
        .sort();
    expect([...new Set(ids(byKind))]).toEqual([...new Set(ids(byTool))]);
  });

  it("filters to one kind, and an unmapped count has no kind to match", () => {
    const view = toolsView(withUnmapped(3), { kind: "config" });
    expect(
      view.groups.flatMap((g) => g.entries).every((e) => e.kind === "config"),
    ).toBe(true);
    expect(view.counts.unmapped).toBe(0);
  });

  it("names the open tool, and ignores one the bundle does not hold", () => {
    expect(toolsView(bundle, { open: "stylelint" }).open?.tool).toBe(
      "stylelint",
    );
    expect(toolsView(bundle, { open: "no-such-tool" }).open).toBeNull();
  });

  it("has no tool left for a filter that matches nothing", () => {
    const view = toolsView(bundle, { tool: "no-such-tool" });
    expect(view.groups).toEqual([]);
    expect(view.counts).toEqual({
      tools: 0,
      entries: 0,
      rules: 0,
      unmapped: 0,
    });
  });
});

describe("the rendering of the same fixture", () => {
  it("lists, per rule, the entries the rule's own list names, with the rungs", () => {
    const view = toolsView(bundle);
    const rows = view.groups.flatMap((g) => [
      ...g.entries,
      ...g.folds.flatMap((f) => f.entries),
    ]);
    for (const rule of bundle.rules) {
      const held = new Set(
        rows
          .filter((e) => e.holders.some((h) => h.rule === rule.id))
          .map((e) => e.id),
      );
      expect([...held].sort()).toEqual([...(rule.bindings ?? [])].sort());
    }
    for (const b of bundle.bindings) {
      const entry = rows.find((e) => e.id === `${b.tool}/${b.key}`);
      expect(
        entry?.holders.some((h) => h.rule === b.rule && h.rung === b.rung),
      ).toBe(true);
    }
  });
});

describe("a value the page cannot show as text", () => {
  it("is a dash with its reason, not a guess at its contents", () => {
    const structured: Bundle = {
      ...bundle,
      bindings: bundle.bindings.map((b, i) =>
        i === 0 ? { ...b, value: { nested: [1, 2] } } : b,
      ),
    };
    const first = bundle.bindings[0];
    const held = toolsView(structured)
      .groups.flatMap((g) => [
        ...g.entries,
        ...g.folds.flatMap((f) => f.entries),
      ])
      .find((e) => e.id === `${first?.tool}/${first?.key}`);
    expect(held?.holders[0]?.value).toEqual({
      kind: "dash",
      reason: "absent.structured",
    });
  });
});

describe("a tool row and a mixed entry", () => {
  it("counts a tool's entries as the distinct entries the bindings name", () => {
    for (const tool of toolsView(bundle).tools) {
      const own = new Set(
        bundle.bindings.filter((b) => b.tool === tool.tool).map((b) => b.key),
      );
      expect(tool.entries).toBe(own.size);
    }
  });

  it("keeps an entry out of the folds when any rule holds it above rung 0", () => {
    const first = bundle.bindings.find((b) => b.rung === 0);
    if (first === undefined)
      throw new Error("the fixture holds a rung-0 entry");
    const other = bundle.rules.find((r) => r.id !== first.rule);
    if (other === undefined) throw new Error("the fixture holds two rules");
    const mixed: Bundle = {
      ...bundle,
      bindings: [...bundle.bindings, { ...first, rule: other.id, rung: 2 }],
    };
    const group = toolsView(mixed).groups.find((g) => g.key === first.tool);
    expect(group?.entries.map((e) => e.key)).toContain(first.key);
    expect(
      group?.folds.flatMap((f) => f.entries.map((e) => e.key)),
    ).not.toContain(first.key);
  });
});

describe("what the view reads", () => {
  it("is listed in the consumer-driven field list, and never the proof grid", () => {
    const recorder = recordReads(bundle);
    toolsView(recorder.value);
    toolsView(recorder.value, {
      tool: "stylelint",
      group: "kind",
      open: "stylelint",
    });
    const reads = recorder.reads();
    expect(reads.filter((f) => f.startsWith("proof_grid"))).toEqual([]);
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
    expect(unlisted(list, "tools", "bundle", reads)).toEqual([]);
  });
});
