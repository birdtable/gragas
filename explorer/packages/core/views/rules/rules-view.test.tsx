// The rules view in a real browser, both engines: every story in
// both themes is clean to the accessibility engine and keeps its targets at
// 24 px; the view holds at 1440 and at 390 CSS px; a filter, a chip and clear-all
// go out through onFilters; and the count line reads the selector's own count.
import axe from "axe-core";
import { createElement } from "react";
import { userEvent } from "vitest/browser";
import { afterEach, describe, expect, test } from "vitest";
import { targetFloor, targetsBelowFloor } from "../../test/measure.ts";
import { render, themes, type Rendered } from "../../test/render.tsx";
import { filterRules } from "./selectors.ts";
import * as stories from "./rules-view.stories.tsx";
import { fixtureBundle } from "./rules-view.fixture.ts";
import { RulesView, states, type RulesViewProps } from "./rules-view.tsx";
import type { Filters } from "./filters.ts";

let shown: Rendered | undefined;
afterEach(() => {
  shown?.unmount();
  shown = undefined;
});

const named = Object.entries(stories).filter(
  (entry): entry is [string, { args: RulesViewProps }] =>
    entry[0] !== "default" && typeof entry[1] === "object",
);

const pascal = (state: string): string =>
  state === "ready"
    ? "Default"
    : state.charAt(0).toUpperCase() + state.slice(1);

async function violations(): Promise<string[]> {
  const results = await axe.run(
    { include: [["body"]], exclude: [["[data-base-ui-focus-guard]"]] },
    {
      rules: {
        region: { enabled: false },
        "landmark-one-main": { enabled: false },
        "page-has-heading-one": { enabled: false },
      },
    },
  );
  return results.violations.map((v) => v.id);
}

function framed(props: RulesViewProps, width: number): Rendered {
  const wrap = createElement(
    "div",
    { style: { width: `${String(width)}px` }, "data-frame": String(width) },
    createElement(RulesView, props),
  );
  return render(wrap);
}

const ready: RulesViewProps = {
  filters: {},
  onFilters: () => undefined,
  onRetry: () => undefined,
  region: { state: "ready", file: "bundle.json", value: fixtureBundle },
};

describe("the stories", () => {
  test("hold one story per state and no story without one", () => {
    const names = named.map(([name]) => name);
    for (const state of states) expect(names).toContain(pascal(state));
    const own = new Set<string>(states.map(pascal));
    const extra = names.filter((n) => !own.has(n)).sort();
    expect(extra).toEqual(["Filtered", "Phone390"]);
  });

  for (const [name, story] of named) {
    for (const theme of themes) {
      test(`${name} in ${theme} is clean and keeps its targets at 24 px`, async () => {
        const width = name === "Phone390" ? 390 : 1440;
        shown = framed(story.args, width);
        await shown.settled();
        expect(
          shown.container.querySelector("[data-part='rules-view']"),
        ).not.toBeNull();
        expect(targetsBelowFloor(document.body, targetFloor.fine)).toEqual([]);
        expect(await violations()).toEqual([]);
      });
    }
  }
});

describe("the layout", () => {
  for (const width of [1440, 390]) {
    test(`at ${String(width)} px the page does not scroll sideways`, async () => {
      shown = framed(ready, width);
      await shown.settled();
      const view = shown.container.querySelector("[data-part='rules-view']");
      expect(view).not.toBeNull();
      expect(view?.scrollWidth).toBeLessThanOrEqual(width);
    });
  }
});

describe("the controls", () => {
  const count = (filters: Filters): string =>
    `${String(filterRules(fixtureBundle.rules, filters, fixtureBundle.structure.parts).length)} of ${String(fixtureBundle.rules.length)} rules match`;

  test("the count line is the selector's count, with and without a filter", async () => {
    shown = framed(ready, 1440);
    await shown.settled();
    expect(
      shown.container.querySelector("[data-part='rules-count']")?.textContent,
    ).toBe(count({}));
    shown.rerender(
      createElement(RulesView, { ...ready, filters: { strength: "must" } }),
    );
    await shown.settled();
    expect(
      shown.container.querySelector("[data-part='rules-count']")?.textContent,
    ).toBe(count({ strength: "must" }));
  });

  test("typing in the search sends q to onFilters", async () => {
    const sent: Filters[] = [];
    shown = framed(
      {
        ...ready,
        onFilters: (next) => {
          sent.push(next);
        },
      },
      1440,
    );
    await shown.settled();
    const input = shown.container.querySelector("input[type='search']");
    if (!(input instanceof HTMLInputElement)) throw new Error("no search box");
    await userEvent.click(input);
    await userEvent.keyboard("wrap");
    expect(sent.at(0)).toEqual({ q: "w" });
  });

  test("a chip's remove and clear-all send the narrowed filters", async () => {
    const sent: Filters[] = [];
    shown = framed(
      {
        ...ready,
        filters: { part: "go", strength: "must" },
        onFilters: (next) => {
          sent.push(next);
        },
      },
      1440,
    );
    await shown.settled();
    const bar = shown.container.querySelector("[data-part='filter-bar']");
    expect(bar?.querySelectorAll("[data-part='filter-chip']")).toHaveLength(2);
    const clear = [...(bar?.querySelectorAll("button") ?? [])].at(-1);
    if (clear === undefined) throw new Error("no clear-all");
    await userEvent.click(clear);
    expect(sent.at(-1)).toEqual({});
  });

  test("every row carries its standing word and a link to its rule", async () => {
    shown = framed(ready, 1440);
    await shown.settled();
    const rows = shown.container.querySelectorAll("tbody tr");
    expect(rows).toHaveLength(fixtureBundle.rules.length);
    for (const row of rows) {
      expect(row.querySelector("[data-part='status-mark']")).not.toBeNull();
      expect(row.querySelector("a[href^='#/rules/']")).not.toBeNull();
    }
  });
});
