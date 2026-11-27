// The rule view's page in a real browser, both engines (gragas#2396): it draws
// each panel from the fixture bundle, a retired rule says so, and each term a
// rule uses links to its entry in the terms view and the terms view links
// back to the rule: followed both ways (gragas#1685, line 1).
import { createElement, type ReactElement } from "react";
import { afterEach, describe, expect, test } from "vitest";
import { render, type Rendered } from "../../test/render.tsx";
import { bundleOf } from "../terms/fixture.ts";
import { termsSearchSchema, TermsRegion } from "../terms/terms.tsx";
import { termsUsedBy } from "../terms/selectors.ts";
import { RuleRegion, ruleSearchSchema, termHref } from "./rule.tsx";

let shown: Rendered | undefined;
afterEach(() => {
  shown?.unmount();
  shown = undefined;
});

const region = {
  state: "ready",
  file: "bundle.json",
  value: bundleOf(),
} as const;
const retry = (): void => undefined;
const draw = (id: string): ReactElement =>
  createElement(RuleRegion, { region, id, onRetry: retry });

describe("the rule page", () => {
  test("takes no search", () => {
    expect(ruleSearchSchema.safeParse({}).success).toBe(true);
    expect(ruleSearchSchema.safeParse({ lang: "fr" }).success).toBe(false);
  });

  test("draws the panels of a rule", () => {
    shown = render(draw("go.errors.1"));
    const parts = [...shown.container.querySelectorAll("[data-part]")].map(
      (e) => e.getAttribute("data-part"),
    );
    for (const panel of [
      "rule-anatomy",
      "rule-terms",
      "rule-bindings",
      "rule-decisions",
      "rule-sources",
      "rule-history",
      "rule-relations",
    ]) {
      expect(parts).toContain(panel);
    }
    expect(shown.container.querySelector("h1")?.textContent).toBe("Rule");
    expect(shown.container.textContent).toContain(
      "Wrap an error with what was being done.",
    );
  });

  test("a rule no one goes by says so", () => {
    shown = render(draw("no.such.rule"));
    expect(shown.container.textContent).toContain(
      "No rule goes by no.such.rule",
    );
  });
});

describe("a term and a rule, linked both ways", () => {
  const bundle = bundleOf();
  const rule = bundle.rules.find((r) => termsUsedBy(bundle, r.id).length > 0);

  test("the fixture has a rule that uses a term", () => {
    expect(rule).toBeDefined();
  });

  test("each term a rule uses links to its entry, and that entry links back", () => {
    if (rule === undefined) throw new Error("no rule uses a term");
    shown = render(draw(rule.id));
    const links = [
      ...shown.container.querySelectorAll('[data-part="rule-term"] a'),
    ].map((a) => a.getAttribute("href"));
    expect(links).toEqual(termsUsedBy(bundle, rule.id).map(termHref));
    shown.unmount();

    for (const link of links) {
      const query = new URLSearchParams((link ?? "").split("?")[1]);
      const search = termsSearchSchema.parse(Object.fromEntries(query));
      expect(search.term).toBeDefined();
      shown = render(
        createElement(TermsRegion, {
          region,
          focus: search.term,
          onRetry: retry,
        }),
      );
      const entry = shown.container.querySelector(
        `#term-${CSS.escape(search.term ?? "")}`,
      );
      expect(entry?.getAttribute("data-focused")).toBe("true");
      expect(
        entry?.querySelector(
          `a[href="#/rules/${encodeURIComponent(rule.id)}"]`,
        ),
      ).not.toBeNull();
      shown.unmount();
    }
    shown = undefined;
  });
});
