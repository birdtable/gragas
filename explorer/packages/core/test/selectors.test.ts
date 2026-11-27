// The pure selectors over the fixture documents, in node, and the
// ban on joining by array position.
import { readdirSync, readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { BundleDocument, ReportDocument, parse } from "@gragas/contract";
import { describe, expect, it } from "vitest";
import {
  currentRuleId,
  indexById,
  joinById,
  previousMajor,
  ruleTable,
  shown,
} from "../index.ts";
import {
  documents,
  field,
  patched,
  type Bundle,
  type Report,
} from "./support.ts";

function bundleAndReport(
  changeReport: (
    doc: ReturnType<ReturnType<typeof documents>["get"]>,
  ) => ReturnType<typeof patched> = (d) => patched(d, {}),
): { bundle: Bundle; report: Report } {
  const all = documents();
  const b = parse(BundleDocument, all.get("bundle"));
  const r = parse(ReportDocument, changeReport(all.get("report")));
  if (b.status !== "ok" || r.status !== "ok")
    throw new Error("the fixtures must parse");
  return { bundle: b.value, report: r.value };
}

describe("indexById and joinById", () => {
  const rows = [
    { id: "a", n: 1 },
    { id: "b", n: 2 },
    { id: "a", n: 3 },
  ];

  it("index by id, keep the first of a repeated id and name the repeat", () => {
    const index = indexById(rows, (row) => row.id);
    expect(index.byId.get("a")?.n).toBe(1);
    expect(index.duplicates).toEqual(["a"]);
  });

  it("join by id whatever the order, and give undefined for no match", () => {
    const right = indexById(
      [
        { id: "b", w: "x" },
        { id: "a", w: "y" },
      ],
      (r) => r.id,
    );
    const joined = joinById(rows.slice(0, 2), (row) => row.id, right.byId);
    expect(joined.map((j) => j.match?.w)).toEqual(["y", "x"]);
    expect(
      joinById([{ id: "z" }], (row) => row.id, right.byId)[0]?.match,
    ).toBeUndefined();
  });
});

describe("the rules table", () => {
  it("joins the report to every rule by id and has no banner for the same release", () => {
    const { bundle, report } = bundleAndReport();
    const table = ruleTable(bundle, report);
    expect(table.rows).toHaveLength(bundle.rules.length);
    expect(table.banner).toBeNull();
    expect(table.rows.every((row) => !row.differs)).toBe(true);
    expect(table.reportOnly).toEqual([]);
  });

  it("with no report, every row stands alone and none is marked", () => {
    const { bundle } = bundleAndReport();
    const table = ruleTable(bundle, null);
    expect(
      table.rows.every(
        (row) => row.sections.length === 0 && !row.disagree && !row.differs,
      ),
    ).toBe(true);
  });

  it("a report from another release is a banner and a mark on each row it lacks", () => {
    const { bundle, report } = bundleAndReport((doc) =>
      patched(doc, {
        release: patched(field(doc, "release"), { version: "9.9.9" }),
        sections: [],
      }),
    );
    const table = ruleTable(bundle, report);
    expect(table.banner).toEqual({
      reportRelease: "9.9.9",
      bundleRelease: bundle.release.version,
    });
    expect(table.rows.every((row) => row.differs)).toBe(true);
  });

  it("is order-blind: a reversed report joins the same", () => {
    const { bundle, report } = bundleAndReport();
    const reversed: Report = {
      ...report,
      sections: report.sections.map((s) => ({
        ...s,
        per_rule: [...s.per_rule].reverse(),
      })),
    };
    expect(
      ruleTable(bundle, reversed).rows.map((r) =>
        r.sections.map((s) => s.entry.rule),
      ),
    ).toEqual(
      ruleTable(bundle, report).rows.map((r) =>
        r.sections.map((s) => s.entry.rule),
      ),
    );
  });
});

describe("a report with a section for each package", () => {
  function twoSections(): { bundle: Bundle; report: Report; id: string } {
    const { bundle, report } = bundleAndReport();
    const first = report.sections[0];
    const entry = first?.per_rule[0];
    if (first === undefined || entry === undefined)
      throw new Error("the fixture report has a section");
    const second = {
      ...first,
      manifest: `sha256:${"0".repeat(64)}`,
      per_rule: [{ ...entry, observation: "not held" as const }],
    };
    return {
      bundle,
      report: { ...report, sections: [first, second] },
      id: entry.rule,
    };
  }

  it("keeps every section's entry on the row and marks the disagreement", () => {
    const { bundle, report, id } = twoSections();
    const row = ruleTable(bundle, report).rows.find((r) => r.rule.id === id);
    expect(row?.sections.map((s) => s.entry.observation)).toEqual([
      "held",
      "not held",
    ]);
    expect(row?.disagree).toBe(true);
  });

  it("marks no other row, and no row of a report whose sections agree", () => {
    const { bundle, report, id } = twoSections();
    const others = ruleTable(bundle, report).rows.filter(
      (r) => r.rule.id !== id,
    );
    expect(others.every((r) => !r.disagree)).toBe(true);
    const single = bundleAndReport();
    expect(
      ruleTable(single.bundle, single.report).rows.every((r) => !r.disagree),
    ).toBe(true);
  });
});

describe("a value nobody read", () => {
  it("shows a dash with its reason", () => {
    expect(shown(null, "no run yet")).toEqual({
      kind: "dash",
      reason: "no run yet",
    });
    expect(shown(0, "no run yet")).toEqual({ kind: "value", value: 0 });
  });
});

// Joins are by id: a selector that indexes by a counter, or reads a
// callback's position argument, is joining by position.
const positional: RegExp[] = [
  /\[\s*(?:i|j|k|n|idx|index|pos)\s*\]/,
  /\(\s*[\w$]+\s*,\s*(?:i|j|k|idx|index|pos)\s*\)\s*=>/,
  /\.entries\(\)/,
  /\bfor\s*\(\s*(?:let|var)\s+\w+\s*=\s*0/,
];

function positionalIn(source: string): string[] {
  return positional.filter((p) => p.test(source)).map(String);
}

describe("no selector joins by array position", () => {
  const folder = fileURLToPath(new URL("../selectors/", import.meta.url));
  const files = readdirSync(folder).filter((name) => name.endsWith(".ts"));

  it("reads the selectors folder", () => {
    expect(files).toContain("by-id.ts");
  });

  it.each(files)("%s names no position", (name) => {
    expect(positionalIn(readFileSync(folder + name, "utf8"))).toEqual([]);
  });

  it("sees each planted position join", () => {
    const plants = [
      "const out = left.map((l, i) => ({ l, r: right[i] }));",
      "for (let i = 0; i < left.length; i++) use(right[i]);",
      "for (const [i, row] of left.entries()) use(row);",
      "left.forEach((l, index) => use(right[index]));",
    ];
    expect(plants.map((plant) => positionalIn(plant).length > 0)).toEqual([
      true,
      true,
      true,
      true,
    ]);
  });
});

describe("currentRuleId and previousMajor", () => {
  it("resolves a rule's id to itself and an alias to the current id", () => {
    const { bundle } = bundleAndReport();
    expect(currentRuleId(bundle, "go.errors.1")).toBe("go.errors.1");
    expect(currentRuleId(bundle, "go.errs.1")).toBe("go.errors.1");
  });

  it("answers null for a name no rule has", () => {
    const { bundle } = bundleAndReport();
    expect(currentRuleId(bundle, "go.nothing.1")).toBeNull();
  });

  it("names the contract of a bundle from an older major, and none for the current", () => {
    const { bundle } = bundleAndReport();
    expect(previousMajor(bundle, 1, "1.0")).toBeNull();
    expect(previousMajor(bundle, 2, "2.0")).toEqual({
      contract: bundle.versions.contract,
      reader: "2.0",
    });
  });
});
