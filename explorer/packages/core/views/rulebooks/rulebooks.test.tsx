// The rulebooks view in a real browser, both engines: the same
// component renders with no provider, in both themes, at 1440 and at 390 pixels
// width; every state it declares draws; the chain shows the parent, this
// rulebook and the import with their pins; the view names no project that pins
// the chain.
import axe from "axe-core";
import { createElement, type ReactElement } from "react";
import { afterEach, describe, expect, test } from "vitest";
import { measure, targetsBelowFloor } from "../../test/measure.ts";
import { render, themes, type Rendered } from "../../test/render.tsx";
import { bundleOf } from "./fixture.ts";
import * as stories from "./rulebooks.stories.tsx";
import { RulebooksRegion, type RulebooksRegionProps } from "./rulebooks.tsx";

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
const args = (name: string): RulebooksRegionProps => {
  const found = Object.entries(table).find(([n]) => n === name)?.[1].args;
  if (found?.region === undefined || found.onRetry === undefined) {
    throw new Error(`${name} has no args`);
  }
  return { region: found.region, onRetry: found.onRetry };
};
const draw = (props: RulebooksRegionProps): ReactElement =>
  createElement(RulebooksRegion, props);

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
          "Rulebooks",
        );
        expect(await violations()).toEqual([]);
        expect(targetsBelowFloor(document.body)).toEqual([]);
      });
    }
  }
});

const boxes = (): Element[] => [
  ...(shown?.container.querySelectorAll('[data-part="rulebooks-box"]') ?? []),
];

describe("the default story", () => {
  test("shows the parent, this rulebook and the import with their pins", () => {
    shown = render(draw(args("Default")));
    expect(boxes().map((b) => b.getAttribute("data-role"))).toEqual([
      "parent",
      "this",
      "import",
    ]);
    const text = boxes().map((b) => b.textContent ?? "");
    expect(text[0]).toContain("fixture-parent");
    expect(text[0]).toContain("0.9.0");
    expect(text[1]).toContain("fixture-base");
    expect(text[1]).toContain("1.2.0");
    expect(text[2]).toContain("fixture-other");
    expect(text[2]).toContain("ui");
  });

  test("the rule map shows kept, tighter, weaker, gone and new", () => {
    shown = render(draw(args("Default")));
    const map = shown.container.querySelector('[data-part="rulebooks-map"]');
    expect(map?.textContent).toContain("fixture-other");
    expect(map?.textContent).toContain(
      "1 kept, 1 tighter, 1 weaker, 1 gone, 0 new",
    );
  });

  test("shows governance and a licence per kind", () => {
    shown = render(draw(args("Default")));
    const governance = shown.container.querySelector(
      '[data-part="rulebooks-governance"]',
    );
    expect(governance?.textContent).toContain("proposals");
    expect(governance?.textContent).toContain("180");
    const licences = shown.container.querySelector(
      '[data-part="rulebooks-licences"]',
    );
    expect(licences?.textContent).toContain("CC-BY-4.0");
    expect(licences?.textContent).toContain("Apache-2.0");
    expect(licences?.textContent).toContain("MIT");
  });

  test("names no project that pins the chain, and draws no archived section for a live rulebook", () => {
    shown = render(draw(args("Default")));
    const text = (shown.container.textContent ?? "").toLowerCase();
    expect(text).not.toContain("project");
    expect(text).not.toContain("withdrawn");
    expect(
      shown.container.querySelector('[data-part="rulebooks-archived"]'),
    ).toBeNull();
  });

  test("an archived rulebook shows its successor, and a dash when it names none", () => {
    const bundle = bundleOf();
    const props = (successor: string | null): RulebooksRegionProps => ({
      region: {
        state: "ready",
        file: "bundle.json",
        value: {
          ...bundle,
          identity: { ...bundle.identity, kind: "archived", successor },
        },
      },
      onRetry: () => undefined,
    });
    shown = render(draw(props("fixture-other")));
    const section = shown.container.querySelector(
      '[data-part="rulebooks-archived"]',
    );
    expect(section?.textContent).toContain("fixture-other");
    shown.unmount();
    shown = render(draw(props(null)));
    expect(
      shown.container.querySelector(
        '[data-part="rulebooks-archived"] [data-part="dash"]',
      ),
    ).not.toBeNull();
  });
});

describe("the other states", () => {
  test("loading says so and draws no chain", () => {
    shown = render(draw(args("Loading")));
    expect(shown.container.querySelector('[role="status"]')).not.toBeNull();
    expect(boxes()).toHaveLength(0);
  });

  test("empty says no rulebook", () => {
    shown = render(draw(args("Empty")));
    expect(shown.container.textContent).toContain("no rulebook");
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

  test("stale keeps the chain and says how old it is", () => {
    shown = render(draw(args("Stale")));
    expect(boxes().length).toBeGreaterThan(0);
    expect(shown.container.textContent).toContain("2 min");
  });
});

describe("at 390 pixels", () => {
  test("the chain does not overflow the width", async () => {
    const frame = document.createElement("div");
    frame.style.width = "390px";
    document.body.append(frame);
    try {
      shown = render(draw(args("Phone390")));
      frame.append(shown.container);
      await shown.settled();
      for (const box of measure(frame, "rulebooks-box")) {
        expect(box.width).toBeLessThanOrEqual(390);
      }
      expect(frame.scrollWidth).toBeLessThanOrEqual(390);
    } finally {
      frame.remove();
    }
  });
});
