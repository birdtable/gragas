// // The terms view in a real browser, both engines: the same
// component renders with no provider, in both themes, at 1440 and at 390
// pixels width; every state it declares draws; the stale mark is a notice with a
// word and a symbol; the fallback mark shows when a translation is missing.
import axe from "axe-core";
import { createElement, type ReactElement } from "react";
import { afterEach, describe, expect, test } from "vitest";
import { measure, targetsBelowFloor } from "../../test/measure.ts";
import { render, themes, type Rendered } from "../../test/render.tsx";
import * as stories from "./terms.stories.tsx";
import { TermsRegion, type TermsRegionProps } from "./terms.tsx";
import { bundleOf } from "./fixture.ts";

let shown: Rendered | undefined;
afterEach(() => {
  shown?.unmount();
  shown = undefined;
});

const table = {
  Default: stories.Default,
  Empty: stories.Empty,
  Error: stories.Error,
  Loading: stories.Loading,
  French: stories.French,
  Phone390: stories.Phone390,
  Stale: stories.Stale,
};
const names = Object.keys(table);
const args = (name: string): TermsRegionProps => {
  const found = Object.entries(table).find(([n]) => n === name)?.[1].args;
  if (found?.region === undefined || found.onRetry === undefined) {
    throw new Error(`${name} has no args`);
  }
  return {
    region: found.region,
    onRetry: found.onRetry,
    ...(found.language === undefined ? {} : { language: found.language }),
  };
};
const draw = (props: TermsRegionProps): ReactElement =>
  createElement(TermsRegion, props);

async function violations(): Promise<string[]> {
  const results = await axe.run(
    { include: [["body"]], exclude: [["[data-base-ui-focus-guard]"]] },
    {
      rules: {
        region: { enabled: false },
        "landmark-one-main": { enabled: false },
      },
    },
  );
  return results.violations.map((v) => v.id);
}

describe("the stories", () => {
  test("name each state the view declares and no other", () => {
    expect(names.sort()).toEqual(
      [
        "Default",
        "Empty",
        "Error",
        "French",
        "Loading",
        "Phone390",
        "Stale",
      ].sort(),
    );
  });

  for (const name of names) {
    for (const theme of themes) {
      test(`${name} in ${theme} is clean and its targets are 24 px`, async () => {
        shown = render(draw(args(name)), { theme });
        await shown.settled();
        expect(shown.container.querySelector("h1")?.textContent).toBe("Terms");
        expect(await violations()).toEqual([]);
        expect(targetsBelowFloor(document.body)).toEqual([]);
      });
    }
  }
});

const terms = (): Element[] => [
  ...(shown?.container.querySelectorAll('[data-part="terms-term"]') ?? []),
];

describe("the default story", () => {
  test("shows every term with its literal, its definition and its users", () => {
    shown = render(draw(args("Default")));
    const bundle = bundleOf();
    expect(terms()).toHaveLength(bundle.terms.length);
    const gate = shown.container.querySelector("#term-gate");
    expect(gate?.textContent).toContain("make check");
    expect(gate?.textContent).toContain(
      "A check that stops a push when it fails.",
    );
  });

  test("a term whose literal is not stated shows a dash with its reason", () => {
    shown = render(draw(args("Default")));
    const dash = shown.container.querySelector('#term-wrap [data-part="dash"]');
    expect(dash).not.toBeNull();
    expect(dash?.getAttribute("title")).toBeTruthy();
  });

  test("links each rule that uses a term to its rule view", () => {
    shown = render(draw(args("Default")));
    const link = shown.container.querySelector(
      '#term-wrap a[href="#/rules/go.errors.1"]',
    );
    expect(link?.textContent).toBe("go.errors.1");
  });

  test("a stale translation is a notice: the word and the symbol, never an alert", () => {
    shown = render(draw(args("Default")));
    const wrap = shown.container.querySelector("#term-wrap");
    const mark = wrap?.querySelector(".gr-terms-mark");
    expect(mark?.textContent).toContain("stale");
    expect(mark?.textContent).toContain("≠");
    expect(shown.container.querySelector('[role="alert"]')).toBeNull();
  });

  test("offers each language as a link that sets it in the address", () => {
    shown = render(draw(args("Default")));
    const hrefs = [
      ...shown.container.querySelectorAll('[data-part="terms-switch"] a'),
    ].map((a) => a.getAttribute("href"));
    expect(hrefs).toEqual(["#/terms", "#/terms?lang=fr"]);
  });
});

describe("the French story", () => {
  test("shows the translation, marks the stale one, and says which language", () => {
    shown = render(draw(args("French")));
    const gate = shown.container.querySelector("#term-gate");
    expect(gate?.textContent).toContain("Un contrôle qui arrête");
    expect(
      gate
        ?.querySelector('[data-part="terms-definition"]')
        ?.getAttribute("data-mark"),
    ).toBe("translated");
    const wrap = shown.container.querySelector("#term-wrap");
    expect(
      wrap
        ?.querySelector('[data-part="terms-definition"]')
        ?.getAttribute("data-mark"),
    ).toBe("stale");
    expect(
      shown.container.querySelector('[aria-current="true"]')?.textContent,
    ).toBe("fr");
  });

  test("a term without a French translation shows the authoritative text with the fallback mark", () => {
    const bundle = bundleOf();
    shown = render(
      draw({
        region: {
          state: "ready",
          file: "bundle.json",
          value: {
            ...bundle,
            terms: bundle.terms.map((t) => ({ ...t, translations: [] })),
          },
        },
        language: "fr",
        onRetry: () => undefined,
      }),
    );
    const first = shown.container.querySelector('[data-part="terms-term"]');
    expect(
      first
        ?.querySelector('[data-part="terms-definition"]')
        ?.getAttribute("data-mark"),
    ).toBe("fallback");
    expect(first?.textContent).toContain("fallback");
    expect(first?.textContent).toContain("↩");
  });
});

describe("the other states", () => {
  test("loading says so and draws no term", () => {
    shown = render(draw(args("Loading")));
    expect(shown.container.querySelector('[role="status"]')).not.toBeNull();
    expect(terms()).toHaveLength(0);
  });

  test("empty says no terms", () => {
    shown = render(draw(args("Empty")));
    expect(shown.container.textContent).toContain("no terms");
  });

  test("a bundle with rules and no term says no terms", () => {
    const bundle = bundleOf();
    shown = render(
      draw({
        region: {
          state: "ready",
          file: "bundle.json",
          value: { ...bundle, terms: [] },
        },
        onRetry: () => undefined,
      }),
    );
    expect(shown.container.textContent).toContain("no terms");
    expect(terms()).toHaveLength(0);
  });

  test("error offers Retry", () => {
    let retried = 0;
    shown = render(
      draw({
        ...args("Error"),
        onRetry: () => {
          retried += 1;
        },
      }),
    );
    shown.container.querySelector("button")?.click();
    expect(retried).toBe(1);
  });

  test("stale keeps the terms and says how old they are", () => {
    shown = render(draw(args("Stale")));
    expect(terms().length).toBeGreaterThan(0);
    expect(shown.container.textContent).toContain("2 min");
  });
});

describe("at 390 pixels", () => {
  test("the terms do not overflow the width", async () => {
    const frame = document.createElement("div");
    frame.style.width = "390px";
    document.body.append(frame);
    try {
      shown = render(draw(args("Phone390")));
      frame.append(shown.container);
      await shown.settled();
      for (const box of measure(frame, "terms-term")) {
        expect(box.width).toBeLessThanOrEqual(390);
      }
      expect(frame.scrollWidth).toBeLessThanOrEqual(390);
    } finally {
      frame.remove();
    }
  });
});
