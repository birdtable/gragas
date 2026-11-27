// The standing view's selectors over the fixture bundle, in node: the
// counts per standing as sums over the rules, the marks beside a standing, both
// groupings, the filters, the dash for rules whose scope names no kind, and the
// agreement with the figures the Markdown page states.
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
import { standingOrder, standingView } from "./selectors.ts";

function fixture(): Bundle {
  const parsed = parse(BundleDocument, documents().get("bundle"));
  if (parsed.status !== "ok") throw new Error("the fixture must parse");
  return parsed.value;
}

const bundle = fixture();
const countOf = (
  segments: readonly { standing: string; count: number }[],
  standing: string,
): number => segments.find((s) => s.standing === standing)?.count ?? -1;

describe("the standings of the fixture bundle", () => {
  const view = standingView(bundle);

  it("sums the rules of each standing, in the order of the tokens, zero kept", () => {
    expect(view.chain.segments.map((s) => s.standing)).toEqual([
      ...standingOrder,
    ]);
    for (const standing of standingOrder) {
      expect(countOf(view.chain.segments, standing)).toBe(
        bundle.rules.filter((r) => r.standing === standing).length,
      );
    }
    expect(view.chain.total).toBe(bundle.rules.length);
    expect(view.chain.segments.reduce((n, s) => n + s.count, 0)).toBe(
      bundle.rules.length,
    );
  });

  it("counts a mark under the standing it sits beside, never in place of it", () => {
    const planned = (mark: string): number =>
      view.chain.marked.find((m) => m.standing === "planned" && m.mark === mark)
        ?.count ?? 0;
    expect(planned("trial")).toBe(2);
    expect(planned("blocked")).toBe(1);
    expect(planned("unproven")).toBe(
      bundle.rules.filter(
        (r) => r.standing === "planned" && r.marks.includes("unproven"),
      ).length,
    );
    expect(countOf(view.chain.segments, "planned")).toBe(9);
  });

  it("shows a trial binding on an enforced rule as an enforced, trial segment", () => {
    const trial: Bundle = {
      ...bundle,
      rules: bundle.rules.map((r) =>
        r.id === "web.forms.1" ? { ...r, marks: ["trial"] } : r,
      ),
    };
    const marked = standingView(trial).chain.marked;
    expect(marked).toContainEqual({
      standing: "enforced",
      mark: "trial",
      count: 1,
    });
  });

  it("makes one bar per part, whose counts add up to the chain's", () => {
    expect(view.by).toBe("part");
    expect(view.groups.map((g) => g.id)).toEqual(["base", "go", "web"]);
    for (const standing of standingOrder) {
      expect(
        view.groups.reduce((n, g) => n + countOf(g.segments, standing), 0),
      ).toBe(countOf(view.chain.segments, standing));
    }
  });
});

describe("the kinds and the filters", () => {
  it("groups by kind and keeps the rules that name none in a dash group with its reason", () => {
    const view = standingView(bundle, { by: "kind" });
    const dashed = view.groups.find((g) => g.key.kind === "dash");
    expect(dashed?.key).toEqual({
      kind: "dash",
      reason: "absent.none-recorded",
    });
    expect(dashed?.total).toBe(
      bundle.rules.filter((r) => r.scope.kinds.length === 0).length,
    );
    expect(view.groups.find((g) => g.id === "service")?.total).toBe(1);
  });

  it("filters by part and by kind, and says what it applied", () => {
    const go = standingView(bundle, { part: "go" });
    expect(go.chain.total).toBe(6);
    expect(go.applied).toEqual([{ name: "part", value: "go" }]);
    const service = standingView(bundle, { kind: "service" });
    expect(service.chain.total).toBe(1);
    expect(service.groups.map((g) => g.id)).toEqual(["go"]);
    expect(
      standingView(bundle, { part: "web", kind: "service" }).groups,
    ).toEqual([]);
  });
});

describe("agreement with the Markdown page", () => {
  it("counts per standing what the M7 page of go.errors states", () => {
    const page = readFileSync(
      fileURLToPath(
        new URL(
          "../../../../../internal/host/assets/contract/fixture/pages/go/errors.md",
          import.meta.url,
        ),
      ),
      "utf8",
    );
    const stated = new Map<string, number>();
    const marks = new Map<string, number>();
    for (const line of page.split("\n")) {
      const row = /^\| go\.errors\.\d+ \|.*\| ([a-z, ]+) \|$/.exec(line);
      const [standing, ...rest] = (row?.[1] ?? "").split(", ");
      if (standing === undefined || standing === "") continue;
      stated.set(standing, (stated.get(standing) ?? 0) + 1);
      for (const mark of rest) {
        marks.set(
          `${standing}/${mark}`,
          (marks.get(`${standing}/${mark}`) ?? 0) + 1,
        );
      }
    }
    const only: Bundle = {
      ...bundle,
      rules: bundle.rules.filter((r) => r.id.startsWith("go.errors.")),
    };
    const view = standingView(only);
    for (const standing of standingOrder) {
      expect(countOf(view.chain.segments, standing)).toBe(
        stated.get(standing) ?? 0,
      );
    }
    for (const m of view.chain.marked) {
      expect(m.count).toBe(marks.get(`${m.standing}/${m.mark}`));
    }
    expect(view.chain.marked).toHaveLength(marks.size);
  });
});

describe("what the view reads", () => {
  it("is listed in the consumer-driven field list, and never the proof grid", () => {
    const recorder = recordReads(bundle);
    standingView(recorder.value);
    standingView(recorder.value, { by: "kind", part: "go", kind: "service" });
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
    expect(unlisted(list, "standing", "bundle", reads)).toEqual([]);
  });
});
