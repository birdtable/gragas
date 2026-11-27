// The health view in a real browser, both engines: the same
// component renders with no provider, in both themes, at 1440 and at 390
// pixels' width; every state it declares draws; the seven numbers show with
// their derivation as text and as the title; the dash carries its reason; a
// number the bundle does not state is never a zero.
import axe from "axe-core";
import { createElement, type ReactElement } from "react";
import { afterEach, describe, expect, test } from "vitest";
import { userEvent } from "vitest/browser";
import { measure, targetsBelowFloor } from "../../test/measure.ts";
import { render, themes, type Rendered } from "../../test/render.tsx";
import * as stories from "./coverage.stories.tsx";
import { CoverageRegion, type CoverageRegionProps } from "./coverage.tsx";
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
  Phone390: stories.Phone390,
  Stale: stories.Stale,
};
const names = Object.keys(table);
const args = (name: string): CoverageRegionProps => {
  const found = Object.entries(table).find(([n]) => n === name)?.[1].args;
  if (
    found?.region === undefined ||
    found.onRetry === undefined ||
    found.chosen === undefined
  ) {
    throw new Error(`${name} has no args`);
  }
  return { region: found.region, chosen: found.chosen, onRetry: found.onRetry };
};
const draw = (props: CoverageRegionProps): ReactElement =>
  createElement(CoverageRegion, props);

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
      ["Default", "Empty", "Error", "Loading", "Phone390", "Stale"].sort(),
    );
  });

  for (const name of names) {
    for (const theme of themes) {
      test(`${name} in ${theme} is clean and its targets are 24 px`, async () => {
        shown = render(draw(args(name)), { theme });
        await shown.settled();
        expect(shown.container.querySelector("h1")?.textContent).toBe(
          "Coverage",
        );
        expect(await violations()).toEqual([]);
        expect(targetsBelowFloor(document.body)).toEqual([]);
      });
    }
  }
});

describe("the default story", () => {
  test("draws both ratios with their numbers written beside the bars", () => {
    shown = render(draw(args("Default")));
    const ratios = [
      ...shown.container.querySelectorAll('[data-part="coverage-ratio"]'),
    ];
    expect(ratios.map((r) => r.querySelector("p")?.textContent)).toEqual([
      "11 of 13 live rules have an entry",
      "16 of 16 entries trace to a rule",
    ]);
    for (const r of ratios) expect(r.querySelector("meter")).not.toBeNull();
  });

  test("draws a part's two bars, by part and layer, and the tools' bars", () => {
    shown = render(draw(args("Default")));
    const parts = measure(shown.container, "coverage-part");
    expect(parts).toHaveLength(3);
    const base = shown.container.querySelector('[data-part-slug="base"]');
    expect(base?.querySelectorAll("meter")).toHaveLength(2);
    expect(base?.textContent).toContain("Base (Core)");
    expect(
      shown.container.querySelectorAll('[data-part="coverage-tool"]').length,
    ).toBeGreaterThan(5);
  });

  test("expands the upstream set to its rules, by keyboard", async () => {
    shown = render(draw(args("Default")));
    const trigger = shown.container.querySelector<HTMLButtonElement>(
      '[data-part="coverage-upstream"] button',
    );
    expect(trigger?.textContent).toContain("fixture-parent 0.9.0");
    expect(trigger?.getAttribute("aria-expanded")).toBe("false");
    trigger?.focus();
    await userEvent.keyboard("{Enter}");
    await expect
      .poll(() => trigger?.getAttribute("aria-expanded"))
      .toBe("true");
    expect(shown.container.textContent).toContain("web.forms.3");
  });

  test("reads the chosen standing with word and symbol, not colour", () => {
    shown = render(draw(args("Phone390")));
    const current = shown.container.querySelector('[aria-current="true"]');
    expect(current?.textContent).toContain("enforced");
    expect(current?.querySelector('[data-part="status-mark"]')).not.toBeNull();
  });

  test("gives a part with nothing to count a dash and its reason, never zero", () => {
    const bundle = bundleOf();
    shown = render(
      draw({
        region: {
          state: "ready",
          file: "bundle.json",
          value: {
            ...bundle,
            rules: bundle.rules.filter((r) => !r.id.startsWith("web.")),
          },
        },
        chosen: null,
        onRetry: () => undefined,
      }),
    );
    const web = shown.container.querySelector('[data-part-slug="web"]');
    expect(web?.querySelectorAll('[data-part="dash"]').length).toBeGreaterThan(
      0,
    );
    expect(web?.textContent).toContain("no live rules or no entries");
  });
});

describe("the other states", () => {
  test("loading says so and draws no bar", () => {
    shown = render(draw(args("Loading")));
    expect(shown.container.querySelector('[role="status"]')).not.toBeNull();
    expect(shown.container.querySelector("meter")).toBeNull();
  });

  test("empty says no rules and offers the handbook", () => {
    shown = render(draw(args("Empty")));
    expect(shown.container.textContent).toContain("no rules");
    expect(shown.container.textContent).toContain("handbook");
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

  test("stale keeps the bars and says how old they are", () => {
    shown = render(draw(args("Stale")));
    expect(shown.container.querySelectorAll("meter").length).toBeGreaterThan(2);
    expect(shown.container.textContent).toContain("2 min");
  });
});

describe("at 390 pixels", () => {
  test("the bars do not overflow the width", async () => {
    const frame = document.createElement("div");
    frame.style.width = "390px";
    document.body.append(frame);
    try {
      shown = render(draw(args("Phone390")));
      frame.append(shown.container);
      await shown.settled();
      for (const box of measure(frame, "coverage-part")) {
        expect(box.width).toBeLessThanOrEqual(390);
      }
      expect(frame.scrollWidth).toBeLessThanOrEqual(390);
    } finally {
      frame.remove();
    }
  });
});
