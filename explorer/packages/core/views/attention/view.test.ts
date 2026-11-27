// The attention view's selectors over the fixture bundle, in node:
// the eight kinds and what puts a rule in each, the clock moving a rule into
// "overdue", the absence of "high override rate", the further lists, the clock
// function, and, by absence, no field the list does not name.
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
import { dayStart, setClock, today } from "../../data/index.ts";
import { documents, type Bundle } from "../../test/support.ts";
import { earlyDay, lateDay } from "./fixture.ts";
import {
  attentionKinds,
  attentionView,
  type AttentionView,
} from "./selectors.ts";

function fixture(): Bundle {
  const parsed = parse(BundleDocument, documents().get("bundle"));
  if (parsed.status !== "ok") throw new Error("the fixture must parse");
  return parsed.value;
}

const bundle = fixture();
const early = attentionView(bundle, earlyDay);

const rulesOf = (view: AttentionView, kind: string): string[] =>
  view.groups.find((g) => g.kind === kind)?.items.map((i) => i.rule) ?? [];

describe("the eight kinds", () => {
  it("are one group each, in the order the page draws them", () => {
    expect(early.groups.map((g) => g.kind)).toEqual([...attentionKinds]);
    expect(attentionKinds).toHaveLength(8);
  });

  it("put every rule with the unproven mark under unproven", () => {
    expect(rulesOf(early, "unproven")).toEqual([
      "base.errors.1",
      "base.errors.2",
      "base.naming.1",
      "base.targets.1",
      "base.targets.2",
      "go.errors.3",
      "go.layout.2",
      "go.layout.3",
    ]);
  });

  it("put every rule with the blocked mark under blocked", () => {
    expect(rulesOf(early, "blocked")).toEqual(["go.errors.1", "go.layout.2"]);
  });

  it("list what a rule lacks: a pair, a source, and dash the decision it cannot judge", () => {
    const missing = new Map(
      early.groups
        .find((g) => g.kind === "missing")
        ?.items.map((i) => [i.rule, i.missing]),
    );
    expect(missing.get("base.targets.1")).toEqual(["source"]);
    expect(missing.get("go.layout.2")).toEqual(["pair", "source"]);
    expect(missing.has("go.errors.1")).toBe(false);
    expect(missing.has("go.layout.1")).toBe(false);
    const decisions = early.groups
      .find((g) => g.kind === "missing")
      ?.items.map((i) => i.decision);
    expect(decisions?.length).toBeGreaterThan(0);
    for (const d of decisions ?? []) {
      expect(d).toEqual({ kind: "dash", reason: "absent.decision-gap" });
    }
  });

  it("do not list a retired rule, and ask for a pair from a may only once it is bound", () => {
    expect(
      early.groups.flatMap((g) => g.items.map((i) => i.rule)),
    ).not.toContain("base.naming.2");
    const unbound = {
      ...bundle,
      rules: bundle.rules.map((r) =>
        r.id === "base.naming.1" ? { ...r, bindings: [] } : r,
      ),
    };
    const listed = early.groups
      .find((g) => g.kind === "missing")
      ?.items.find((i) => i.rule === "base.naming.1");
    expect(listed?.missing).toEqual(["pair"]);
    const items = attentionView(unbound, earlyDay).groups.find(
      (g) => g.kind === "missing",
    )?.items;
    expect(items?.find((i) => i.rule === "base.naming.1")).toBeUndefined();
  });

  it("count a rule once however many kinds it is in", () => {
    expect(early.itemCount).toBe(21);
    expect(early.ruleCount).toBe(12);
  });

  it("give an item its owner, or a dash with its reason", () => {
    const item = early.groups
      .find((g) => g.kind === "missing")
      ?.items.find((i) => i.rule === "go.errors.2");
    expect(item?.owner).toEqual({
      kind: "dash",
      reason: "absent.none-recorded",
    });
    const owned = early.groups
      .find((g) => g.kind === "blocked")
      ?.items.find((i) => i.rule === "go.errors.1");
    expect(owned?.owner).toEqual({ kind: "value", value: "alex" });
  });
});

describe("the clock", () => {
  it("moves a rule into overdue when the day passes its review-by", () => {
    expect(rulesOf(early, "overdue")).toEqual([]);
    const late = attentionView(bundle, lateDay);
    expect(rulesOf(late, "overdue")).toContain("go.layout.1");
    expect(rulesOf(late, "overdue")).toHaveLength(11);
    const item = late.groups
      .find((g) => g.kind === "overdue")
      ?.items.find((i) => i.rule === "go.layout.1");
    expect(item?.date).toEqual({ kind: "value", value: "2027-03-01" });
  });

  it("does not call the day of the review-by overdue", () => {
    expect(rulesOf(attentionView(bundle, "2027-03-01"), "overdue")).toEqual([]);
  });

  it("moves a planned rule past its zero-by", () => {
    expect(
      rulesOf(attentionView(bundle, "2027-06-01"), "past-zero-by"),
    ).toEqual([]);
    expect(
      rulesOf(attentionView(bundle, "2027-06-02"), "past-zero-by"),
    ).toEqual(["go.errors.3"]);
  });

  it("reads the same list from the same bundle and day", () => {
    expect(attentionView(bundle, lateDay)).toEqual(
      attentionView(bundle, lateDay),
    );
  });

  it("is one function a test replaces, and puts back", () => {
    const restore = setClock(() => Date.parse("2027-05-05T23:59:59Z"));
    try {
      expect(today()).toBe("2027-05-05");
      expect(rulesOf(attentionView(bundle, today()), "overdue")).toHaveLength(
        11,
      );
    } finally {
      restore();
    }
    expect(today()).toBe(new Date(Date.now()).toISOString().slice(0, 10));
    expect(dayStart("2027-05-05")).toBe(Date.parse("2027-05-05T00:00:00Z"));
  });
});

describe("a trial past its window", () => {
  it("is one that shipped in at least as many releases as the governance allows", () => {
    expect(rulesOf(early, "trial-overrun")).toEqual([]);
    const tight = {
      ...bundle,
      identity: {
        ...bundle.identity,
        governance: { ...bundle.identity.governance, trial_releases: 1 },
      },
    };
    const view = attentionView(tight, earlyDay);
    expect(rulesOf(view, "trial-overrun")).toEqual(["go.errors.3"]);
    expect(
      view.groups.find((g) => g.kind === "trial-overrun")?.items[0]?.trial,
    ).toEqual({
      shipped: 1,
      allowed: 1,
    });
  });
});

describe("a rule a linter now holds that still sits in review text", () => {
  it("is absent from the fixture, whose renderings agree with the rules", () => {
    expect(rulesOf(early, "review-held")).toEqual([]);
    expect(early.staleRenderings).toEqual([]);
  });

  it("is an enforced rule whose agent or review rendering still shows it as planned", () => {
    const stale = {
      ...bundle,
      rendering_manifest: bundle.rendering_manifest.map((m) => ({
        ...m,
        target: "agent",
        renderings: m.renderings.map((r) => ({
          ...r,
          rules: r.rules.map((x) =>
            x.rule === "go.errors.1"
              ? { ...x, standing: "planned" as const }
              : x,
          ),
        })),
      })),
    };
    const view = attentionView(stale, earlyDay);
    expect(rulesOf(view, "review-held")).toEqual(["go.errors.1"]);
    expect(
      view.groups.find((g) => g.kind === "review-held")?.items[0],
    ).toMatchObject({
      shown: "planned",
      path: "pages/fr/go/errors.md",
    });
    expect(view.staleRenderings.map((s) => s.rule)).toEqual([
      "go.errors.1",
      "go.errors.1",
    ]);
  });

  it("is not raised for a rendering of another target, which is only stale", () => {
    const pages = {
      ...bundle,
      rendering_manifest: bundle.rendering_manifest.map((m) => ({
        ...m,
        renderings: m.renderings.map((r) => ({
          ...r,
          rules: r.rules.map((x) =>
            x.rule === "go.errors.1"
              ? { ...x, standing: "planned" as const }
              : x,
          ),
        })),
      })),
    };
    const view = attentionView(pages, earlyDay);
    expect(rulesOf(view, "review-held")).toEqual([]);
    expect(view.staleRenderings).toHaveLength(2);
  });
});

describe("high override rate", () => {
  it("is never computed: the bundle carries no batches, and the group says so", () => {
    const group = early.groups.find((g) => g.kind === "override-rate");
    expect(group?.items).toEqual([]);
    expect(group?.needsBatches).toBe(true);
  });
});

describe("the further lists", () => {
  it("hold the open conflicts of the bundle", () => {
    expect(early.conflicts).toHaveLength(1);
    expect(early.conflicts[0]).toMatchObject({
      rule: { kind: "value", value: "go.errors.1" },
      entity: "golangci-lint/wrapcheck",
    });
  });
});

describe("the filter", () => {
  it("leaves one group, and the counts stay the book's", () => {
    const view = attentionView(bundle, earlyDay, { kind: "blocked" });
    expect(view.shownGroups.map((g) => g.kind)).toEqual(["blocked"]);
    expect(view.itemCount).toBe(early.itemCount);
  });
});

describe("a book where every rule is proposed", () => {
  it("says so, so the whole book is read at once", () => {
    const proposed = {
      ...bundle,
      rules: bundle.rules
        .filter((r) => r.id === "go.errors.2")
        .map((r) => ({ ...r })),
    };
    expect(attentionView(proposed, earlyDay).allProposed).toBe(true);
    expect(early.allProposed).toBe(false);
  });
});

describe("no page of its own", () => {
  it("is in no rendering the release shipped: attention is computed on demand and would be stale tomorrow", () => {
    const paths = bundle.rendering_manifest.flatMap((m) =>
      m.renderings.map((r) => r.path),
    );
    expect(paths.filter((p) => p.includes("attention"))).toEqual([]);
  });
});

describe("the bundle's own words", () => {
  it("reads only the bundle, and only fields the list names", () => {
    const recorder = recordReads(bundle);
    attentionView(recorder.value, lateDay);
    const reads = recorder.reads();
    expect(reads.length).toBeGreaterThan(8);
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
    expect(unlisted(list, "attention", "bundle", reads)).toEqual([]);
  });
});
