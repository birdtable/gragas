// The branded ids and the closed unions (ts.type-safety.6, ts.type-safety.7).
import { describe, expect, it } from "vitest";
import {
  BindingId,
  DecisionId,
  ReleaseId,
  RuleId,
  SourceId,
  TermId,
  closedUnions,
  exhaustive,
  type RuleStanding,
} from "../src/main.js";

describe("the branded ids", () => {
  it("each parses its own shape and refuses another's", () => {
    expect(RuleId.safeParse("go.errors.1").success).toBe(true);
    expect(RuleId.safeParse("go-errors").success).toBe(false);
    expect(BindingId.safeParse("go.errors.1").success).toBe(true);
    for (const slug of [DecisionId, SourceId, TermId]) {
      expect(slug.safeParse("wrap-errors").success).toBe(true);
      expect(slug.safeParse("go.errors.1").success).toBe(false);
    }
    expect(ReleaseId.safeParse("1.2.0").success).toBe(true);
    expect(ReleaseId.safeParse("1.2").success).toBe(false);
  });

  it("keep their brand in the type, so one is not another", () => {
    const rule = RuleId.parse("go.errors.1");
    const same: RuleId = rule;
    expect(same).toBe("go.errors.1");
  });
});

describe("the closed unions", () => {
  it("name every member the schema has", () => {
    expect(closedUnions.status).toEqual([
      "proposed",
      "trial",
      "active",
      "retired",
    ]);
    expect(closedUnions.standing).toContain("enforced");
    expect(closedUnions.mark).toEqual(["same", "tighter", "weaker", "none"]);
    expect(closedUnions.observation).toEqual([
      "held",
      "not held",
      "not observed",
    ]);
    expect(closedUnions.proofOutcome).toEqual([
      "pass",
      "fail",
      "not run",
      "reviewed",
    ]);
    expect(closedUnions.level).toEqual(["fixture", "pair", "project"]);
  });

  it("make a switch exhaustive, so a missed member does not compile", () => {
    const label = (standing: RuleStanding): string => {
      switch (standing) {
        case "proposed":
        case "planned":
        case "advisory":
          return "not enforced";
        case "enforced":
        case "reviewed":
          return "enforced";
        case "retired":
          return "retired";
        default:
          return exhaustive(standing);
      }
    };
    expect(closedUnions.standing.map(label)).toHaveLength(
      closedUnions.standing.length,
    );
  });
});
