// The relations view in a real browser, both engines: it renders
// with no provider, in both themes, at 1440 and at 390 pixels; every state it
// declares draws; the list carries exactly the edges the graph draws; an entity
// walks to the graph around it; a control adds one hop and removes it again; a
// kind or an entity the bundle lacks says so.
import axe from "axe-core";
import { createElement, type ReactElement } from "react";
import { afterEach, describe, expect, test } from "vitest";
import { measure, targetsBelowFloor } from "../../test/measure.ts";
import { render, themes, type Rendered } from "../../test/render.tsx";
import { bundleOf } from "./fixture.ts";
import * as stories from "./relations.stories.tsx";
import { RelationsRegion, type RelationsRegionProps } from "./relations.tsx";

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
const args = (name: string): RelationsRegionProps => {
  const found = Object.entries(table).find(([n]) => n === name)?.[1].args;
  if (
    found?.region === undefined ||
    found.kind === undefined ||
    found.id === undefined ||
    found.onRetry === undefined
  ) {
    throw new Error(`${name} has no args`);
  }
  return {
    region: found.region,
    kind: found.kind,
    id: found.id,
    expanded: found.expanded,
    onExpand: found.onExpand,
    onRetry: found.onRetry,
  };
};
const draw = (props: RelationsRegionProps): ReactElement =>
  createElement(RelationsRegion, props);
const ready = (): RelationsRegionProps["region"] => ({
  state: "ready",
  file: "bundle.json",
  value: bundleOf(),
});

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
          "Relations",
        );
        expect(await violations()).toEqual([]);
        expect(targetsBelowFloor(document.body)).toEqual([]);
      });
    }
  }
});

const attrs = (selector: string, root: ParentNode): string[] =>
  [...root.querySelectorAll(selector)]
    .map(
      (e) =>
        `${e.getAttribute("data-from") ?? ""} ${e.getAttribute("data-label") ?? ""} ${e.getAttribute("data-to") ?? ""}`,
    )
    .sort();

describe("the default story", () => {
  test("the list carries exactly the edges the graph draws", () => {
    shown = render(draw(args("Default")));
    const drawn = attrs('[data-part="relations-edge"]', shown.container);
    expect(drawn.length).toBeGreaterThan(0);
    expect(attrs('[data-part="relations-row"]', shown.container)).toEqual(
      drawn,
    );
  });

  test("every entity is a link to the graph around it", () => {
    shown = render(draw(args("Default")));
    const links = [
      ...shown.container.querySelectorAll('[data-part="relations-entity"] a'),
    ];
    expect(links.length).toBeGreaterThan(0);
    for (const link of links) {
      expect(link.getAttribute("href")).toMatch(/^#\/relations\//);
    }
  });

  test("a control adds one hop around its node and takes it away again", () => {
    let written: readonly string[] | undefined;
    shown = render(
      draw({
        ...args("Default"),
        onExpand: (keys) => {
          written = keys;
        },
      }),
    );
    const entity = shown.container.querySelector(
      '[data-part="relations-entity"]:not([data-key="rule:go.errors.1"])',
    );
    const key = entity?.getAttribute("data-key");
    entity?.querySelector("button")?.click();
    expect(written).toEqual([key]);
    shown.unmount();
    shown = render(
      draw({
        ...args("Default"),
        expanded: key === null || key === undefined ? [] : [key],
        onExpand: (keys) => {
          written = keys;
        },
      }),
    );
    shown.container
      .querySelector(
        `[data-part="relations-entity"][data-key="${key ?? ""}"] button`,
      )
      ?.click();
    expect(written).toEqual([]);
  });
});

describe("an address the bundle cannot answer", () => {
  test("a kind that is not one says so", () => {
    shown = render(
      draw({ ...args("Default"), region: ready(), kind: "banana", id: "x" }),
    );
    expect(shown.container.textContent).toContain("banana");
    expect(
      shown.container.querySelector('[data-part="relations-graph"]'),
    ).toBeNull();
  });

  test("an entity the bundle lacks says so", () => {
    shown = render(
      draw({
        ...args("Default"),
        region: ready(),
        kind: "rule",
        id: "no.such.1",
      }),
    );
    expect(shown.container.textContent).toContain("no.such.1");
    expect(
      shown.container.querySelector('[data-part="relations-graph"]'),
    ).toBeNull();
  });
});

describe("the other states", () => {
  test("loading says so and draws no graph", () => {
    shown = render(draw(args("Loading")));
    expect(shown.container.querySelector('[role="status"]')).not.toBeNull();
    expect(
      shown.container.querySelector('[data-part="relations-graph"]'),
    ).toBeNull();
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

  test("stale keeps the graph and says how old it is", () => {
    shown = render(draw(args("Stale")));
    expect(
      shown.container.querySelector('[data-part="relations-graph"]'),
    ).not.toBeNull();
    expect(shown.container.textContent).toContain("2 min");
  });
});

describe("at 390 pixels", () => {
  test("the graph does not overflow the width", async () => {
    const frame = document.createElement("div");
    frame.style.width = "390px";
    document.body.append(frame);
    try {
      shown = render(draw(args("Phone390")));
      frame.append(shown.container);
      await shown.settled();
      for (const box of measure(frame, "relations-graph")) {
        expect(box.width).toBeLessThanOrEqual(390);
      }
      expect(frame.scrollWidth).toBeLessThanOrEqual(390);
    } finally {
      frame.remove();
    }
  });
});
