// The catalogue test: every part in the registry has a living story
// in each of its states, every story renders through the harness in both themes,
// the accessibility engine finds nothing on it against the shrink-only list, and
// every pointer target on it is at least 24 CSS pixels. A part with a state that
// has no story fails here, and so does a story no state names.
import axe from "axe-core";
import { createElement, type ReactElement } from "react";
import { afterEach, describe, expect, test } from "vitest";
import {
  measure,
  partsIn,
  targetFloor,
  targetsBelowFloor,
} from "../test/measure.ts";
import { render, themes, type Rendered } from "../test/render.tsx";
import { catalogue, type StoryModule } from "./catalogue.ts";

// The accessibility list only shrinks: the most findings all the parts' stories
// may excuse between them. Lower it as an excuse goes; never raise it.
const KNOWN_CEILING = 0;

type Component = (props: Record<string, unknown>) => ReactElement;
type Known = Readonly<{ rule: string; reason: string }>;

const isRecord = (value: unknown): value is Record<string, unknown> =>
  typeof value === "object" && value !== null;
const isComponent = (value: unknown): value is Component =>
  typeof value === "function";

function readMeta(module: StoryModule): {
  states: string[];
  component: Component;
  known: Known[];
} {
  const meta = module["default"];
  if (!isRecord(meta)) throw new Error("a stories file has no default meta");
  const { states, component, parameters } = meta;
  if (!Array.isArray(states) || !isComponent(component)) {
    throw new Error("the meta names no states or no component");
  }
  const a11y = isRecord(parameters) ? parameters["a11y"] : undefined;
  const known = isRecord(a11y) ? a11y["known"] : undefined;
  return {
    states: states.map(String),
    component,
    known: Array.isArray(known)
      ? known.filter(isRecord).map((k) => ({
          rule: String(k["rule"]),
          reason: String(k["reason"]),
        }))
      : [],
  };
}

function readStories(
  module: StoryModule,
): Map<string, Record<string, unknown>> {
  const stories = new Map<string, Record<string, unknown>>();
  for (const [name, story] of Object.entries(module)) {
    if (name === "default") continue;
    if (!isRecord(story) || !isRecord(story["args"])) {
      throw new Error(`${name} is no story: it has no args`);
    }
    stories.set(name, story["args"]);
  }
  return stories;
}

const pascal = (state: string): string =>
  state
    .split("-")
    .map((word) => word.charAt(0).toUpperCase() + word.slice(1))
    .join("");

const parts = Object.keys(catalogue);

async function violations(): Promise<string[]> {
  // The library's focus guards (WebKit gives them a button role and no name)
  // are its own inert edge elements, not a part's markup.
  const results = await axe.run(
    {
      include: [["body"]],
      exclude: [["[data-base-ui-focus-guard]"]],
    },
    {
      rules: {
        // A part is a fragment of a page, not a page.
        region: { enabled: false },
        "landmark-one-main": { enabled: false },
        "page-has-heading-one": { enabled: false },
      },
    },
  );
  return results.violations.map((v) => v.id);
}

let shown: Rendered | undefined;
afterEach(() => {
  shown?.unmount();
  shown = undefined;
});

describe("the registry", () => {
  test("holds the seventeen parts", () => {
    expect(parts).toHaveLength(17);
  });

  test("excuses no more than the shrink-only list allows", () => {
    const known = parts.flatMap((part) => {
      const module = catalogue[part];
      return module === undefined ? [] : readMeta(module).known;
    });
    expect(known.length).toBeLessThanOrEqual(KNOWN_CEILING);
  });
});

for (const part of parts) {
  const module = catalogue[part];
  if (module === undefined) continue;
  const meta = readMeta(module);
  const stories = readStories(module);

  describe(`${part}`, () => {
    test("has a story for each state and no story without a state", () => {
      expect([...stories.keys()].sort()).toEqual(
        meta.states.map(pascal).sort(),
      );
    });

    for (const [name, args] of stories) {
      for (const theme of themes) {
        test(`${name} in ${theme} renders, is clean and has targets of 24 px`, async () => {
          shown = render(createElement(meta.component, args), { theme });
          await shown.settled();
          // The story's part is on the page, and every part on the page is one
          // the registry holds.
          expect(partsIn(document.body)).toContain(part);
          for (const used of partsIn(document.body)) {
            expect(parts).toContain(used);
          }
          expect(measure(document.body, part).length).toBeGreaterThan(0);
          expect(targetsBelowFloor(document.body, targetFloor.fine)).toEqual(
            [],
          );
          const found = (await violations()).filter(
            (id) => !meta.known.some((k) => k.rule === id),
          );
          expect(found).toEqual([]);
        });
      }
    }
  });
}

describe("the target check", () => {
  test("fails a plant that shrinks a target below 24 px", () => {
    const style = document.createElement("style");
    style.textContent =
      "[data-part='plant'] { min-width: 0; min-height: 0; width: 10px; height: 10px; padding: 0; }";
    document.head.append(style);
    try {
      shown = render(
        createElement("button", { "data-part": "plant", type: "button" }, "x"),
      );
      expect(targetsBelowFloor(shown.container)).toHaveLength(1);
    } finally {
      style.remove();
    }
  });

  test("passes a target that holds its floor", () => {
    shown = render(
      createElement(
        "button",
        { "data-part": "plant", className: "gr-target", type: "button" },
        "x",
      ),
    );
    expect(targetsBelowFloor(shown.container)).toEqual([]);
  });
});
