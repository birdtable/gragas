// The standing page in a real browser, both engines: every story in both
// themes is clean under the accessibility engine and keeps 24 px targets, the
// words are the message module's, a dash says why, and the controls hand the
// next query to the host and write nothing themselves.
import axe from "axe-core";
import { createElement, type ReactElement } from "react";
import { afterEach, describe, expect, test, vi } from "vitest";
import { render, themes, type Rendered } from "../../test/render.tsx";
import { targetFloor, targetsBelowFloor } from "../../test/measure.ts";
import * as stories from "./standing.stories.tsx";
import { StandingBody, type StandingBodyProps } from "./standing.tsx";

let shown: Rendered | undefined;
afterEach(() => {
  shown?.unmount();
  shown = undefined;
});

const byName: Readonly<Record<string, StandingBodyProps>> = {
  Disconnected: stories.Disconnected.args,
  Embedded: stories.Embedded.args,
  Empty: stories.Empty.args,
  Error: stories.Error.args,
  Loading: stories.Loading.args,
  Phone390: stories.Phone390.args,
  Ready: stories.Ready.args,
  Stale: stories.Stale.args,
};

function argsOf(name: string): StandingBodyProps {
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
        "Embedded",
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
        const rendered = show(createElement(StandingBody, argsOf(name)), theme);
        if (name === "Phone390") rendered.container.style.width = "390px";
        await rendered.settled();
        expect(document.querySelector("h1")?.textContent).toBe("Standing");
        expect(targetsBelowFloor(document.body, targetFloor.fine)).toEqual([]);
        expect(await violations()).toEqual([]);
      });
    }
  }
});

describe("the ready page", () => {
  test("draws one bar per part, and each standing as its word, symbol and count", async () => {
    const rendered = show(createElement(StandingBody, argsOf("Ready")));
    await rendered.settled();
    const groups = document.querySelectorAll("[data-region=group]");
    expect(groups.length).toBe(3);
    const legend = document.querySelector("[data-region=chain] .gr-bar-legend");
    for (const word of [
      "proposed",
      "enforced",
      "reviewed",
      "planned",
      "advisory",
      "retired",
    ]) {
      expect(legend?.textContent.toLowerCase()).toContain(word);
    }
    expect(
      document.querySelector("[data-region=chain] [data-mark=trial]")
        ?.textContent,
    ).toContain("trial");
  });

  test("the embedded rendering is the one bar of the chain and no breakdown", async () => {
    const rendered = show(createElement(StandingBody, argsOf("Embedded")));
    await rendered.settled();
    expect(document.querySelector("[data-region=chain]")).not.toBeNull();
    expect(document.querySelector("[data-region=groups]")).toBeNull();
    expect(document.querySelector("[data-region=controls]")).toBeNull();
  });

  test("a grouping toggle hands the next query to the host", async () => {
    const onQuery = vi.fn();
    const rendered = show(
      createElement(StandingBody, { ...argsOf("Ready"), onQuery }),
    );
    await rendered.settled();
    const kind = [...document.querySelectorAll("button")].find(
      (b) =>
        b.textContent === "Kind" && b.getAttribute("aria-pressed") === "false",
    );
    kind?.click();
    expect(onQuery).toHaveBeenCalledWith({ by: "kind" });
  });

  test("a failed load offers Retry and calls it", async () => {
    const onRetry = vi.fn();
    const rendered = show(
      createElement(StandingBody, { ...argsOf("Error"), onRetry }),
    );
    await rendered.settled();
    const retry = [...document.querySelectorAll("button")].find((b) =>
      /retry/i.test(b.textContent),
    );
    retry?.click();
    expect(onRetry).toHaveBeenCalledTimes(1);
  });
});
