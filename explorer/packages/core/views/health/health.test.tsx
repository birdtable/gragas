// The health view in a real browser, both engines: the same
// component renders with no provider, in both themes, at 1440 and at 390
// pixels' width; every state it declares draws; the seven numbers show with
// their derivation as text and as the title; the dash carries its reason; a
// number the bundle does not state is never a zero.
import axe from "axe-core";
import { createElement, type ReactElement } from "react";
import { afterEach, describe, expect, test } from "vitest";
import { measure, targetsBelowFloor } from "../../test/measure.ts";
import { render, themes, type Rendered } from "../../test/render.tsx";
import * as stories from "./health.stories.tsx";
import { HealthRegion, type HealthRegionProps } from "./health.tsx";
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
const args = (name: string): HealthRegionProps => {
  const found = Object.entries(table).find(([n]) => n === name)?.[1].args;
  if (found?.region === undefined || found.onRetry === undefined) {
    throw new Error(`${name} has no args`);
  }
  return { region: found.region, onRetry: found.onRetry };
};
const draw = (props: HealthRegionProps): ReactElement =>
  createElement(HealthRegion, props);

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
        expect(shown.container.querySelector("h1")?.textContent).toBe("Health");
        expect(await violations()).toEqual([]);
        expect(targetsBelowFloor(document.body)).toEqual([]);
      });
    }
  }
});

describe("the default story", () => {
  test("shows seven tiles in the first of two labelled sets", () => {
    shown = render(draw(args("Default")));
    const sets = shown.container.querySelectorAll('[data-part="health-set"]');
    expect(sets).toHaveLength(2);
    expect([...sets].map((s) => s.querySelector("h2")?.textContent)).toEqual([
      "Its own rules",
      "The resolved chain",
    ]);
    expect(measure(sets[0] ?? document.body, "health-tile")).toHaveLength(7);
  });

  test("writes each derivation as text, and as the tile's title", () => {
    shown = render(draw(args("Default")));
    const tiles = shown.container.querySelectorAll('[data-part="health-tile"]');
    expect(tiles).toHaveLength(7);
    for (const tile of tiles) {
      const note = tile.querySelector(".gr-tile-note")?.textContent;
      expect(note).toBeTruthy();
      expect(tile.getAttribute("title")).toBe(note);
    }
  });

  test("says the numbers as the release computed them, in S29's order", () => {
    shown = render(draw(args("Default")));
    const own = shown.container.querySelector('[data-part="health-set"]');
    const values = [...(own?.querySelectorAll(".gr-tile-value") ?? [])].map(
      (v) => v.textContent,
    );
    expect(values).toEqual([
      "84.6%",
      "100%",
      "75%",
      "27.3%",
      "100%",
      "11",
      "1 day",
    ]);
  });

  test("states the proof pass rate with what ran", () => {
    shown = render(draw(args("Default")));
    expect(shown.container.textContent).toContain(
      "3 of 4 that ran; 13 not run",
    );
  });

  test("gives a dash its reason, never a zero", () => {
    shown = render(draw(args("Default")));
    const text = shown.container.textContent ?? "";
    expect(text).toContain("the contract has no field for it yet");
    const dashes = shown.container.querySelectorAll('[data-part="dash"]');
    expect(dashes.length).toBeGreaterThan(0);
    for (const d of dashes) expect(d.getAttribute("title")).toBeTruthy();
  });

  test("names no project", () => {
    shown = render(draw(args("Default")));
    expect(shown.container.textContent).not.toMatch(/larkspur|project/i);
  });
});

describe("the other states", () => {
  test("loading says so and draws no tile", () => {
    shown = render(draw(args("Loading")));
    expect(shown.container.querySelector('[role="status"]')).not.toBeNull();
    expect(
      shown.container.querySelector('[data-part="health-tile"]'),
    ).toBeNull();
  });

  test("empty says no rules", () => {
    shown = render(draw(args("Empty")));
    expect(shown.container.textContent).toContain("no rules");
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

  test("stale keeps the numbers and says how old they are", () => {
    shown = render(draw(args("Stale")));
    expect(
      shown.container.querySelectorAll('[data-part="health-tile"]'),
    ).toHaveLength(7);
    expect(shown.container.textContent).toContain("2 min");
  });

  test("a base rulebook has one set and no set heading", () => {
    const bundle = bundleOf();
    shown = render(
      draw({
        region: {
          state: "ready",
          file: "bundle.json",
          value: {
            ...bundle,
            identity: {
              ...bundle.identity,
              kind: "base",
              extends: null,
              imports: [],
            },
          },
        },
        onRetry: () => undefined,
      }),
    );
    expect(
      shown.container.querySelectorAll('[data-part="health-set"]'),
    ).toHaveLength(1);
    expect(
      shown.container.querySelector("section h2#health-set-own"),
    ).toBeNull();
  });
});

describe("at 390 pixels", () => {
  test("the tiles do not overflow the width", async () => {
    const frame = document.createElement("div");
    frame.style.width = "390px";
    document.body.append(frame);
    try {
      shown = render(draw(args("Phone390")));
      frame.append(shown.container);
      await shown.settled();
      for (const box of measure(frame, "health-tile")) {
        expect(box.width).toBeLessThanOrEqual(390);
      }
      expect(frame.scrollWidth).toBeLessThanOrEqual(390);
    } finally {
      frame.remove();
    }
  });
});
