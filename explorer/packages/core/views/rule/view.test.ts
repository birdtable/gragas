// The rule view's selectors over the fixture bundle, in node: the
// rule's standing, its binding rows with proof cells from the binding's own
// summary, the rung-6 question, the retired panel, the alias redirect, the
// history and the figures the Markdown page states.
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
import { defaultMessages } from "../../messages/index.ts";
import { documents, type Bundle } from "../../test/support.ts";
import { ruleView, type RuleView } from "./selectors.ts";

function fixture(): Bundle {
  const parsed = parse(BundleDocument, documents().get("bundle"));
  if (parsed.status !== "ok") throw new Error("the fixture must parse");
  return parsed.value;
}

function found(bundle: Bundle, id: string): RuleView {
  const result = ruleView(bundle, id);
  if (result.kind !== "found") throw new Error(`${id} is ${result.kind}`);
  return result.view;
}

const bundle = fixture();

describe("a rule of the fixture bundle", () => {
  const view = found(bundle, "go.errors.1");

  it("shows its statement, strength, status, standing and marks", () => {
    expect(view).toMatchObject({
      id: "go.errors.1",
      statement: "Wrap an error with what was being done.",
      strength: "must",
      status: "active",
      standing: "enforced",
      marks: ["blocked"],
      aliases: ["go.errs.1"],
    });
    expect(view.anatomy.definition).toBe(view.statement);
    expect(view.anatomy.reasoning).toEqual({
      kind: "value",
      value: "An error says what failed and what was being done.",
    });
  });

  // Contract gap 1: pros, cons and the example pair are not fields of a rule.
  it("draws the anatomy the contract has no field for as a dash with its reason", () => {
    for (const part of ["pros", "cons", "example"] as const) {
      expect(view.anatomy[part]).toEqual({
        kind: "dash",
        reason: "absent.not-in-bundle",
      });
    }
    const keys: readonly string[] = Object.keys(defaultMessages);
    expect(keys).toContain("absent.not-in-bundle");
  });

  // Contract gap 3: one proof cell per binding, not one per tested tool version.
  it("lists each binding with its proof cell from the binding's own summary", () => {
    expect(view.bindings.map((b) => b.entry)).toEqual([
      "golangci-lint/wrapcheck",
      "golangci-lint/errorlint",
    ]);
    const wrapcheck = view.bindings[0];
    expect(wrapcheck).toMatchObject({
      id: "go.errors.1/golangci-lint/wrapcheck",
      rung: 1,
      runPoint: "push",
      remedy: {
        kind: "value",
        value: "Wrap the error with fmt.Errorf and %w.",
      },
      pairPath: { kind: "value", value: "pkg/a.go" },
      proof: {
        kind: "value",
        value: [{ result: "pass", level: "pair", toolVersion: "2.1.0" }],
      },
      review: null,
    });
    expect(wrapcheck?.destination).toEqual({
      route: "relations",
      kind: "binding",
      id: "go.errors.1/golangci-lint/wrapcheck",
    });
  });

  it("shows a remedy nobody wrote as a dash", () => {
    const errorlint = view.bindings[1];
    expect(errorlint?.proof.kind).toBe("value");
    expect(errorlint?.remedy).toEqual({
      kind: "dash",
      reason: "absent.none-recorded",
    });
  });

  it("shows a binding nobody ran as a dash that names why, never as a pass", () => {
    expect(found(bundle, "base.errors.1").bindings[0]?.proof).toEqual({
      kind: "dash",
      reason: "proof.reason.no-pair",
    });
  });

  // Contract gap 4: adapters_used carries no waiver syntax.
  it("shows no waiver syntax the bundle does not record, and says so", () => {
    for (const row of view.bindings) {
      expect(row.waiver).toEqual({
        kind: "dash",
        reason: "absent.not-in-bundle",
      });
    }
  });

  it("lists the decisions that cite it, the sources it rests on and its links", () => {
    expect(view.decisions.map((d) => d.slug)).toEqual([
      "wrap-errors",
      "wrap-errors-again",
    ]);
    expect(view.sources.map((s) => s.slug)).toEqual([
      "effective-go",
      "tool-manual",
      "error-study",
    ]);
    expect(view.relations.map((r) => `${r.kind} ${r.rule}`)).toEqual([
      "see-also go.layout.1",
      "conflicts-with go.layout.2",
      "see-also base.errors.1",
      "refines go.errors.2",
    ]);
    expect(view.relationsLink).toEqual({
      route: "relations",
      kind: "rule",
      id: "go.errors.1",
    });
    expect(
      view.translations.map((t) => `${t.language} ${t.field} ${t.stale}`),
    ).toEqual(["fr statement false", "fr reasoning true"]);
  });

  it("tells the history in order with who, when and the plan digest", () => {
    expect(view.history.map((h) => [h.kind, h.decision])).toEqual([
      ["init", { kind: "dash", reason: "absent.none-recorded" }],
      ["promote", { kind: "value", value: "wrap-errors" }],
      ["promote", { kind: "value", value: "wrap-errors-again" }],
    ]);
    // The status each step moved to is in an untyped record: a dash until the contract types it
    // (contract gap 2).
    for (const h of view.history) {
      expect(h.status).toEqual({
        kind: "dash",
        reason: "absent.not-in-bundle",
      });
    }
    for (const h of view.history) {
      expect(h.who).toBe("alex");
      expect(h.plan).toMatch(/^sha256:[0-9a-f]{64}$/);
      expect(h.when).toMatch(/^2026-/);
    }
    expect(view.history.map((h) => h.number)).toEqual([1, 2, 5]);
  });
});

describe("a rung-6 binding", () => {
  it("shows its review question, when it applies and the reviewed cell", () => {
    const row = found(bundle, "go.layout.1").bindings[0];
    expect(row?.review).toEqual({
      question: { kind: "value", value: "a person looks" },
      applies: "review",
    });
    expect(row?.proof).toEqual({
      kind: "value",
      value: [{ result: "reviewed", level: "pair", toolVersion: "none" }],
    });
  });

  // Contract gap 5: no field says the question is bindings[].value.
  it("shows a dash for a question the binding does not carry", () => {
    const changed: Bundle = {
      ...bundle,
      bindings: bundle.bindings.map((b) =>
        b.rule === "go.layout.1" ? { ...b, value: null } : b,
      ),
    };
    expect(found(changed, "go.layout.1").bindings[0]?.review?.question).toEqual(
      {
        kind: "dash",
        reason: "absent.not-in-bundle",
      },
    );
  });
});

describe("a retired rule and an alias", () => {
  it("shows the retired panel and links its replacement", () => {
    const view = found(bundle, "base.naming.2");
    expect(view.status).toBe("retired");
    expect(view.retired).toEqual({
      replacement: {
        kind: "value",
        value: { route: "rule", id: "base.naming.1" },
      },
    });
    expect(found(bundle, "go.errors.1").retired).toBeNull();
  });

  it("redirects an alias to the current id, and says a stranger is missing", () => {
    expect(ruleView(bundle, "go.errs.1")).toEqual({
      kind: "redirect",
      to: "go.errors.1",
    });
    expect(ruleView(bundle, "no.such.1")).toEqual({
      kind: "missing",
      id: "no.such.1",
    });
  });
});

describe("a not-run proof", () => {
  it("is a dash that names why it did not run", () => {
    const changed: Bundle = {
      ...bundle,
      bindings: bundle.bindings.map((b) =>
        b.rule === "go.errors.1" && b.key === "wrapcheck" && b.proof !== null
          ? {
              ...b,
              proof: { ...b.proof, result: "not run", reason: "no pair" },
            }
          : b,
      ),
    };
    expect(found(changed, "go.errors.1").bindings[0]?.proof).toEqual({
      kind: "dash",
      reason: "proof.reason.no-pair",
    });
  });
});

describe("the Markdown page of the same fixture", () => {
  it("states the figures the view shows", () => {
    const page = readFileSync(
      fileURLToPath(
        new URL(
          "../../../../../internal/host/assets/contract/fixture/pages/go/errors.md",
          import.meta.url,
        ),
      ),
      "utf8",
    );
    const rows = [
      ...page.matchAll(
        /^\| (go\.errors\.\d) \| ([^|]+) \| (\w+) \| ([^|]+) \|$/gm,
      ),
    ];
    expect(rows.map((m) => m[1])).toEqual([
      "go.errors.1",
      "go.errors.2",
      "go.errors.3",
    ]);
    for (const m of rows) {
      const view = found(bundle, m[1] ?? "");
      expect([
        view.statement,
        view.strength,
        [view.standing, ...view.marks].join(", "),
      ]).toEqual([(m[2] ?? "").trim(), m[3], (m[4] ?? "").trim()]);
    }
  });
});

describe("what the view reads", () => {
  it("is listed in the consumer-driven field list, and never the proof grid", () => {
    const recorder = recordReads(bundle);
    for (const rule of recorder.value.rules) ruleView(recorder.value, rule.id);
    ruleView(recorder.value, "go.errs.1");
    ruleView(recorder.value, "no.such.1");
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
    expect(unlisted(list, "rule", "bundle", reads)).toEqual([]);
  });
});
