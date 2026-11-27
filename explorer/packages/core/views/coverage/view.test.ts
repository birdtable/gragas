// The coverage view's selectors over the fixture bundle, in node:
// the ratios equal the bundle's numerators and denominators and agree with the
// health numbers the release claims; the parts and tools group what the bundle
// states; a standing narrows both; the upstream set is one unit that expands to
// its rules; a part with nothing to count is a dash, never a zero.
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
import { coverageView } from "./selectors.ts";

function fixture(): Bundle {
  const parsed = parse(BundleDocument, documents().get("bundle"));
  if (parsed.status !== "ok") throw new Error("the fixture must parse");
  return parsed.value;
}

const bundle = fixture();
const view = coverageView(bundle, null);

describe("the fixture", () => {
  it("counts rules held over live rules, as the bundle's list says", () => {
    expect(view.held).toEqual({
      n: bundle.coverage.rules_to_bindings.length,
      of: 13,
    });
  });

  it("counts entries traced over the entries the bindings name", () => {
    expect(view.explained).toEqual({
      n: bundle.coverage.bindings_to_rules.length,
      of: bundle.coverage.bindings_to_rules.length,
    });
  });

  it("agrees with the health numbers the release claims", () => {
    const claimed = (name: string): number | undefined =>
      bundle.health.numbers.find((n) => n.name === name)?.value;
    expect(Number((view.held.n / view.held.of).toFixed(4))).toBe(
      claimed("rules-held"),
    );
    expect(view.explained.n / view.explained.of).toBe(
      claimed("entries-explained"),
    );
  });

  it("gives each part its two ratios and its six standings", () => {
    expect(view.parts.map((p) => p.slug)).toEqual(["base", "go", "web"]);
    const web = view.parts.find((p) => p.slug === "web");
    expect(web?.rules).toEqual({ kind: "value", value: { n: 1, of: 3 } });
    expect(web?.segments.map((s) => s.standing)).toHaveLength(6);
    expect(
      view.parts.flatMap((p) => p.segments).reduce((n, s) => n + s.count, 0),
    ).toBe(bundle.rules.length);
  });

  it("groups the entries by tool and sums to all of them", () => {
    expect(view.tools.reduce((n, t) => n + t.entries.of, 0)).toBe(
      view.explained.of,
    );
    expect(view.tools.map((t) => t.tool)).toEqual(
      [...view.tools.map((t) => t.tool)].sort(),
    );
  });

  it("has one upstream set that expands to the imported rules", () => {
    expect(view.upstream.kind).toBe("value");
    const set = view.upstream.kind === "value" ? view.upstream.value : null;
    expect(set?.rulebook).toBe("fixture-parent");
    expect(set?.rules.map((r) => r.id)).toEqual([
      "base.errors.1",
      "web.forms.3",
    ]);
  });
});

describe("a chosen standing", () => {
  it("counts only the rules of that standing and the entries they own", () => {
    const enforced = coverageView(bundle, "enforced");
    expect(enforced.chosen).toBe("enforced");
    expect(enforced.held).toEqual({ n: 2, of: 2 });
    expect(enforced.upstream).toEqual({
      kind: "dash",
      reason: "absent.none-recorded",
    });
  });

  it("shows a dash with its reason for a part with nothing of it, never zero", () => {
    const advisory = coverageView(bundle, "advisory");
    const go = advisory.parts.find((p) => p.slug === "go");
    expect(go?.rules).toEqual({ kind: "dash", reason: "absent.none-in-part" });
    expect(go?.entries).toEqual({
      kind: "dash",
      reason: "absent.none-in-part",
    });
  });
});

describe("a rulebook with no rules", () => {
  it("has nothing held and nothing explained", () => {
    const none = coverageView({ ...bundle, rules: [], bindings: [] }, null);
    expect(none.held).toEqual({ n: 0, of: 0 });
    expect(none.tools).toEqual([]);
    expect(none.parts.every((p) => p.rules.kind === "dash")).toBe(true);
  });
});

describe("the contract", () => {
  it("is read only through fields the list names", () => {
    const recorder = recordReads(bundle);
    coverageView(recorder.value, "planned");
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
    expect(unlisted(list, "coverage", "bundle", reads)).toEqual([]);
  });
});
