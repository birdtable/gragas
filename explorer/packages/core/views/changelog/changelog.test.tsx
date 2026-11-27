// The changelog view in a real browser, both engines: the same
// component renders with no provider, in both themes, at 1440 and at 390 pixels
// width; every state it declares draws; a change with no action reads as none;
// nothing on the view applies anything.
import axe from "axe-core";
import { createElement, type ReactElement } from "react";
import { afterEach, describe, expect, test } from "vitest";
import { measure, targetsBelowFloor } from "../../test/measure.ts";
import { render, themes, type Rendered } from "../../test/render.tsx";
import * as stories from "./changelog.stories.tsx";
import { ChangelogRegion, type ChangelogRegionProps } from "./changelog.tsx";
import { bareBundle, bundleOf } from "./fixture.ts";

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
  Retired: stories.Retired,
  Phone390: stories.Phone390,
  Stale: stories.Stale,
};
const names = Object.keys(table);
const args = (name: string): ChangelogRegionProps => {
  const found = Object.entries(table).find(([n]) => n === name)?.[1].args;
  if (found?.region === undefined || found.onRetry === undefined) {
    throw new Error(`${name} has no args`);
  }
  return {
    region: found.region,
    onRetry: found.onRetry,
    ...(found.filters === undefined ? {} : { filters: found.filters }),
  };
};
const draw = (props: ChangelogRegionProps): ReactElement =>
  createElement(ChangelogRegion, props);

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
        "Retired",
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
          "Changelog",
        );
        expect(await violations()).toEqual([]);
        expect(targetsBelowFloor(document.body)).toEqual([]);
      });
    }
  }
});

const releases = (): Element[] => [
  ...(shown?.container.querySelectorAll('[data-part="changelog-release"]') ??
    []),
];
const changes = (): Element[] => [
  ...(shown?.container.querySelectorAll('[data-part="changelog-change"]') ??
    []),
];

describe("the default story", () => {
  test("shows the releases newest first, each with its bump kind", () => {
    shown = render(draw(args("Default")));
    expect(releases().map((r) => r.id)).toEqual([
      "release-1.2.0",
      "release-1.1.0",
      "release-1.0.0",
    ]);
    expect(releases().map((r) => r.querySelector("h2")?.textContent)).toEqual([
      "Release 1.2.0",
      "Release 1.1.0",
      "Release 1.0.0",
    ]);
    expect(
      releases()[2]?.querySelector('[data-part="changelog-bump"]')?.textContent,
    ).toContain("major release");
  });

  test("shows one row per change, with its action and its remedy", () => {
    shown = render(draw(args("Default")));
    expect(changes()).toHaveLength(bundleOf().changelog.length);
    const wrap = releases()[2]?.querySelector('[data-part="changelog-change"]');
    expect(wrap?.textContent).toContain(
      "Wrap every error a service returns with what was being done.",
    );
    expect(wrap?.textContent).toContain(
      "Wrap the error with fmt.Errorf and %w.",
    );
  });

  test("reads a change with no action as none", () => {
    shown = render(draw(args("Default")));
    const none = shown.container.querySelector(
      '[data-part="changelog-action"][data-action="none"]',
    );
    expect(none?.textContent).toContain("none");
  });

  test("gives a change's diff link, and a dash with its reason where there is none", () => {
    shown = render(draw(args("Default")));
    const links = [
      ...shown.container.querySelectorAll('[data-part="changelog-diff"] a'),
    ];
    expect(links.map((a) => a.getAttribute("href"))).toEqual([
      "https://example.com/fixture-base/compare/1.0.0...1.1.0",
    ]);
    expect(
      shown.container
        .querySelector('[data-part="changelog-diff"] [data-part="dash"]')
        ?.getAttribute("title"),
    ).toBeTruthy();
  });

  test("links a retired rule to its rule view and says it is retired", () => {
    shown = render(draw(args("Default")));
    const link = shown.container.querySelector(
      'a[href="#/rules/base.naming.2"]',
    );
    expect(link?.textContent).toContain("base.naming.2");
    expect(
      link?.closest('[data-part="changelog-change"]')?.textContent,
    ).toContain("retired");
  });

  test("says nothing is applied here, and offers no button that applies", () => {
    shown = render(draw(args("Default")));
    expect(
      shown.container.querySelector('[data-part="changelog-nothing-applied"]')
        ?.textContent,
    ).toContain("Nothing is applied here");
    expect(shown.container.querySelector("button")).toBeNull();
  });

  test("offers each filter as a link that sets it in the address", () => {
    shown = render(draw(args("Default")));
    const hrefs = [
      ...shown.container.querySelectorAll('[data-part="changelog-filter"] a'),
    ].map((a) => a.getAttribute("href"));
    expect(hrefs).toContain("#/changelog");
    expect(hrefs).toContain("#/changelog?change=replaced");
    expect(hrefs).toContain("#/changelog?action=none");
  });
});

describe("the retired story", () => {
  test("shows only the replaced change, and a way to clear the filter", () => {
    shown = render(draw(args("Retired")));
    expect(changes()).toHaveLength(1);
    expect(releases()).toHaveLength(1);
    expect(
      shown.container.querySelector('[aria-current="true"]')?.textContent,
    ).toBe("replaced");
    expect(shown.container.textContent).toContain("Clear filters");
  });

  test("a filter that matches nothing says so", () => {
    shown = render(
      draw({
        region: { state: "ready", file: "bundle.json", value: bundleOf() },
        filters: { change: "retired" },
        onRetry: () => undefined,
      }),
    );
    expect(shown.container.textContent).toContain("no change matches");
  });
});

describe("the other states", () => {
  test("loading says so and draws no release", () => {
    shown = render(draw(args("Loading")));
    expect(shown.container.querySelector('[role="status"]')).not.toBeNull();
    expect(releases()).toHaveLength(0);
  });

  test("empty says no changes", () => {
    shown = render(draw(args("Empty")));
    expect(shown.container.textContent).toContain("no changes");
  });

  test("a bundle with no change recorded says no changes", () => {
    shown = render(
      draw({
        region: { state: "ready", file: "bundle.json", value: bareBundle() },
        onRetry: () => undefined,
      }),
    );
    expect(shown.container.textContent).toContain("no changes");
    expect(releases()).toHaveLength(0);
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

  test("stale keeps the releases and says how old they are", () => {
    shown = render(draw(args("Stale")));
    expect(releases().length).toBeGreaterThan(0);
    expect(shown.container.textContent).toContain("2 min");
  });
});

describe("at 390 pixels", () => {
  test("the changes do not overflow the width", async () => {
    const frame = document.createElement("div");
    frame.style.width = "390px";
    document.body.append(frame);
    try {
      shown = render(draw(args("Phone390")));
      frame.append(shown.container);
      await shown.settled();
      for (const box of measure(frame, "changelog-change")) {
        expect(box.width).toBeLessThanOrEqual(390);
      }
      expect(frame.scrollWidth).toBeLessThanOrEqual(390);
    } finally {
      frame.remove();
    }
  });
});
