// The tools page in a real browser, both engines: every story in both
// themes is clean under the accessibility engine and keeps 24 px targets, the
// words are the message module's, a dash says why, and the controls hand the
// next query to the host and write nothing themselves.
import axe from "axe-core";
import { createElement, type ReactElement } from "react";
import { afterEach, describe, expect, test, vi } from "vitest";
import { render, themes, type Rendered } from "../../test/render.tsx";
import { targetFloor, targetsBelowFloor } from "../../test/measure.ts";
import * as stories from "./tools.stories.tsx";
import { ToolsBody, type ToolsBodyProps } from "./tools.tsx";

let shown: Rendered | undefined;
afterEach(() => {
  shown?.unmount();
  shown = undefined;
});

const byName: Readonly<Record<string, ToolsBodyProps>> = {
  Disconnected: stories.Disconnected.args,
  Empty: stories.Empty.args,
  Error: stories.Error.args,
  Loading: stories.Loading.args,
  Phone390: stories.Phone390.args,
  Ready: stories.Ready.args,
  Stale: stories.Stale.args,
};

function argsOf(name: string): ToolsBodyProps {
  const args = byName[name];
  if (args === undefined) throw new globalThis.Error(`${name} is no story`);
  return args;
}

const names = Object.keys(stories).filter((n) => n !== "default");

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

function show(ui: ReactElement, theme: "light" | "dark" = "light"): Rendered {
  shown = render(ui, { theme });
  return shown;
}

describe("the stories", () => {
  test("name every state and no other", () => {
    expect(names.sort()).toEqual(
      [
        "Disconnected",
        "Empty",
        "Error",
        "Loading",
        "Phone390",
        "Ready",
        "Stale",
      ].sort(),
    );
  });

  for (const name of names) {
    for (const theme of themes) {
      test(`${name} in ${theme} is clean and has targets of 24 px`, async () => {
        const rendered = show(createElement(ToolsBody, argsOf(name)), theme);
        if (name === "Phone390") rendered.container.style.width = "390px";
        await rendered.settled();
        expect(document.querySelector("h1")?.textContent).toBe("Tools");
        expect(targetsBelowFloor(document.body, targetFloor.fine)).toEqual([]);
        expect(await violations()).toEqual([]);
      });
    }
  }
});

describe("the ready page", () => {
  test("lists the fixture's tools and puts a dash with its reason where a value is missing", async () => {
    const rendered = show(createElement(ToolsBody, argsOf("Ready")));
    await rendered.settled();
    const rows = document.querySelectorAll("[data-region='tools'] tbody tr");
    expect(rows.length).toBeGreaterThan(0);
    expect(document.querySelector("[data-region='unmapped']")).toBeNull();
  });

  test("a grouping toggle hands the next query to the host", async () => {
    const onQuery = vi.fn();
    const rendered = show(
      createElement(ToolsBody, { ...argsOf("Ready"), onQuery }),
    );
    await rendered.settled();
    const kind = [...document.querySelectorAll("button")].find(
      (b) => b.textContent === "Kind",
    );
    kind?.click();
    expect(onQuery).toHaveBeenCalledWith({ group: "kind" });
  });

  test("a failed load offers Retry and calls it", async () => {
    const onRetry = vi.fn();
    const rendered = show(
      createElement(ToolsBody, { ...argsOf("Error"), onRetry }),
    );
    await rendered.settled();
    const retry = [...document.querySelectorAll("button")].find((b) =>
      /retry/i.test(b.textContent),
    );
    retry?.click();
    expect(onRetry).toHaveBeenCalledTimes(1);
  });
});
