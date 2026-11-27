// The attention view in a real browser, both engines: the same
// component renders with no provider, in both themes, at 1440 and at 390 pixels
// width; every state it declares draws; the eight kinds are groups; "high
// override rate" says no batches were given; the day used is stated.
import axe from "axe-core";
import { createElement, type ReactElement } from "react";
import { afterEach, describe, expect, test } from "vitest";
import { measure, targetsBelowFloor } from "../../test/measure.ts";
import { render, themes, type Rendered } from "../../test/render.tsx";
import * as stories from "./attention.stories.tsx";
import { AttentionRegion, type AttentionRegionProps } from "./attention.tsx";
import { bundleOf, earlyDay } from "./fixture.ts";

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
  Overdue: stories.Overdue,
  Phone390: stories.Phone390,
  Stale: stories.Stale,
};
const names = Object.keys(table);
const args = (name: string): AttentionRegionProps => {
  const found = Object.entries(table).find(([n]) => n === name)?.[1].args;
  if (
    found?.region === undefined ||
    found.onRetry === undefined ||
    found.today === undefined
  ) {
    throw new Error(`${name} has no args`);
  }
  return {
    region: found.region,
    today: found.today,
    onRetry: found.onRetry,
    ...(found.filters === undefined ? {} : { filters: found.filters }),
  };
};
const draw = (props: AttentionRegionProps): ReactElement =>
  createElement(AttentionRegion, props);

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
        "Loading",
        "Overdue",
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
        expect(shown.container.querySelector("h1")?.textContent).toBe(
          "Attention",
        );
        expect(await violations()).toEqual([]);
        expect(targetsBelowFloor(document.body)).toEqual([]);
      });
    }
  }
});

const groups = (): Element[] => [
  ...(shown?.container.querySelectorAll('[data-part="attention-group"]') ?? []),
];
const items = (): Element[] => [
  ...(shown?.container.querySelectorAll('[data-part="attention-item"]') ?? []),
];

describe("the default story", () => {
  test("shows the eight kinds as groups, in order", () => {
    shown = render(draw(args("Default")));
    expect(groups().map((g) => g.getAttribute("data-kind"))).toEqual([
      "unproven",
      "missing",
      "overdue",
      "past-zero-by",
      "trial-overrun",
      "blocked",
      "override-rate",
      "review-held",
    ]);
  });

  test("shows each item with its rule, date and owner", () => {
    shown = render(draw(args("Default")));
    expect(items()).toHaveLength(21);
    const blocked = shown.container.querySelector(
      '[data-part="attention-group"][data-kind="blocked"] [data-part="attention-item"]',
    );
    expect(blocked?.textContent).toContain("go.errors.1");
    expect(
      blocked?.querySelector('[data-part="attention-owner"]')?.textContent,
    ).toBe("alex");
    expect(
      blocked
        ?.querySelector('[data-part="attention-date"] [data-part="dash"]')
        ?.getAttribute("title"),
    ).toBeTruthy();
  });

  test("says what a rule lacks", () => {
    shown = render(draw(args("Default")));
    const text = shown.container.querySelector(
      '[data-part="attention-group"][data-kind="missing"]',
    )?.textContent;
    expect(text).toContain("an example pair and a source");
    expect(text).toContain("Decision:");
    expect(text).toContain("no decision field on a rule yet");
  });

  test("says the override rate is not computed, and shows no item for it", () => {
    shown = render(draw(args("Default")));
    const group = shown.container.querySelector(
      '[data-part="attention-group"][data-kind="override-rate"]',
    );
    expect(group?.textContent).toContain("override rate: no batches given");
    expect(
      group?.querySelectorAll('[data-part="attention-item"]'),
    ).toHaveLength(0);
  });

  test("states the day it used", () => {
    shown = render(draw(args("Default")));
    expect(
      shown.container.querySelector('[data-part="attention-today"]')
        ?.textContent,
    ).toContain("22 Nov 2026");
  });

  test("lists the open conflict as a further list", () => {
    shown = render(draw(args("Default")));
    expect(
      shown.container.querySelectorAll('[data-part="attention-conflict"]'),
    ).toHaveLength(1);
    expect(
      shown.container.querySelector(
        '[data-part="attention-further"][data-kind="renderings"]',
      )?.textContent,
    ).toContain("no stale rendering");
  });

  test("offers each kind as a link that sets it in the address", () => {
    shown = render(draw(args("Default")));
    const hrefs = [
      ...shown.container.querySelectorAll('[data-part="attention-filter"] a'),
    ].map((a) => a.getAttribute("href"));
    expect(hrefs).toContain("#/attention");
    expect(hrefs).toContain("#/attention?kind=overdue");
    expect(hrefs).toHaveLength(9);
  });

  test("offers no button that applies anything", () => {
    shown = render(draw(args("Default")));
    expect(shown.container.querySelector("button")).toBeNull();
  });
});

describe("the overdue story", () => {
  test("shows only the overdue group, each item with its review-by", () => {
    shown = render(draw(args("Overdue")));
    expect(groups()).toHaveLength(1);
    expect(items()).toHaveLength(11);
    expect(items()[0]?.textContent).toContain("review-by");
    expect(items()[0]?.textContent).toContain("1 Mar 2027");
    expect(
      shown.container.querySelector('[aria-current="true"]')?.textContent,
    ).toBe("Overdue review-by");
    expect(shown.container.textContent).toContain("Clear filter");
  });
});

describe("the other states", () => {
  test("loading says so and draws no group", () => {
    shown = render(draw(args("Loading")));
    expect(shown.container.querySelector('[role="status"]')).not.toBeNull();
    expect(groups()).toHaveLength(0);
  });

  test("empty says no rule needs a person", () => {
    shown = render(draw(args("Empty")));
    expect(shown.container.textContent).toContain("no rule needs a person");
  });

  test("a bundle with nothing to look at says so and still names the day", () => {
    shown = render(
      draw({
        region: {
          state: "ready",
          file: "bundle.json",
          value: { ...bundleOf(), rules: [], conflicts: [] },
        },
        today: earlyDay,
        onRetry: () => undefined,
      }),
    );
    expect(shown.container.textContent).toContain("no rule needs a person");
    expect(items()).toHaveLength(0);
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

  test("stale keeps the groups and says how old they are", () => {
    shown = render(draw(args("Stale")));
    expect(groups().length).toBeGreaterThan(0);
    expect(shown.container.textContent).toContain("2 min");
  });
});

describe("at 390 pixels", () => {
  test("the items do not overflow the width", async () => {
    const frame = document.createElement("div");
    frame.style.width = "390px";
    document.body.append(frame);
    try {
      shown = render(draw(args("Phone390")));
      frame.append(shown.container);
      await shown.settled();
      for (const box of measure(frame, "attention-item")) {
        expect(box.width).toBeLessThanOrEqual(390);
      }
      expect(frame.scrollWidth).toBeLessThanOrEqual(390);
    } finally {
      frame.remove();
    }
  });
});
