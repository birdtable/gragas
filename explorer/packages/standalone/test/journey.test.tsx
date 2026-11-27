// The shell in a real browser, both engines: the dev build's own
// router, chrome and the core's block reader (documentBlocks, the page's one
// reader), with the fixture bundle the Go side ships
// inlined as script blocks by this harness. Keys go through the keyboard.
import {
  createExplorerClient,
  documentBlocks,
  type DataSource,
} from "@gragas/core";
import { RouterProvider } from "@tanstack/react-router";
import { createRoot, type Root } from "react-dom/client";
import { userEvent } from "vitest/browser";
import { afterEach, describe, expect, test } from "vitest";
import bundleText from "../../../../internal/host/assets/contract/fixture/bundle.json?raw";
import reportText from "../../../../internal/host/assets/contract/fixture/report-clean.json?raw";
import { createAppRouter, type AppRouter } from "../src/router.tsx";
import { viewTitles } from "../src/views.ts";
import { say } from "../src/say.ts";

let root: Root | undefined;
let router: AppRouter | undefined;
let host: HTMLElement | undefined;
const blocks: HTMLScriptElement[] = [];

function inline(file: string, text: string): void {
  const block = document.createElement("script");
  block.type = "application/json";
  block.dataset["file"] = file;
  block.textContent = text;
  document.head.append(block);
  blocks.push(block);
}

function open(hash: string, bundle: string = bundleText): void {
  inline("bundle.json", bundle);
  inline("report.json", reportText);
  window.location.hash = hash;
  const source: DataSource = {
    base: "http://127.0.0.1:9/",
    inline: documentBlocks(document),
  };
  router = createAppRouter({ client: createExplorerClient(), source });
  host = document.createElement("div");
  document.body.append(host);
  root = createRoot(host);
  root.render(<RouterProvider router={router} />);
}

afterEach(() => {
  root?.unmount();
  router?.history.destroy();
  host?.remove();
  for (const b of blocks.splice(0)) b.remove();
  root = undefined;
  router = undefined;
  host = undefined;
  window.location.hash = "";
});

const heading = (): string | null =>
  document.querySelector("main h1")?.textContent ?? null;
const titleOf = (key: keyof typeof viewTitles): string => say(viewTitles[key]);

async function seen(text: string): Promise<void> {
  await expect.poll(heading).toBe(text);
}

const nav = [
  ["rules", "/rules"],
  ["rulebooks", "/rulebooks"],
  ["tools", "/tools"],
  ["coverage", "/coverage"],
  ["standing", "/standing"],
  ["health", "/health"],
  ["changelog", "/changelog"],
  ["terms", "/terms"],
  ["attention", "/attention"],
] as const;

describe("navigation by keyboard", () => {
  test("the bare address lands on the rules table", async () => {
    open("");
    await seen(titleOf("rules"));
    expect(window.location.hash).toBe("#/rules");
  });

  test("every listed view opens with Enter, takes focus on its heading and sets the title", async () => {
    open("#/rules");
    await seen(titleOf("rules"));
    for (const [id, path] of nav.slice(1)) {
      const link = document.querySelector<HTMLAnchorElement>(
        `nav a[href$="#${path}"]`,
      );
      expect(link === null ? `no link for ${id}` : "linked").toBe("linked");
      link?.focus();
      await userEvent.keyboard("{Enter}");
      await seen(titleOf(id));
      expect(window.location.hash).toBe(`#${path}`);
      await expect.poll(() => document.activeElement?.tagName).toBe("H1");
      expect(document.title).toBe(say("shell.title", { view: titleOf(id) }));
    }
  });

  test("the rule and relations addresses open their pages", async () => {
    open("#/relations/rule/go.errors.1");
    await seen(titleOf("relations"));
    await expect
      .poll(
        () =>
          document.querySelector("[data-part='relations-center']")?.textContent,
      )
      .toBe("Around rule go.errors.1");
  });
});

describe("addresses", () => {
  test("a name a rule used to go by resolves to the current rule", async () => {
    open("#/rules/go.errs.1");
    await seen(titleOf("rule"));
    expect(window.location.hash).toBe("#/rules/go.errors.1");
    await expect
      .poll(() => document.querySelector("[data-part='rule-id']")?.textContent)
      .toBe("go.errors.1");
  });

  test("a name no rule has gone by is the not-found page", async () => {
    open("#/rules/no.such.rule");
    await seen(say("shell.not-found.heading"));
    expect(heading()).toBe(say("shell.not-found.heading"));
  });

  test("an unknown route is the not-found page", async () => {
    open("#/nowhere");
    await seen(say("shell.not-found.heading"));
    expect(document.title).toBe(
      say("shell.title", { view: say("shell.not-found.heading") }),
    );
  });

  test("a search key the route does not name shows the error view", async () => {
    open("#/rules?bogus=1");
    await seen(say("part.error.heading"));
    expect(document.querySelector("main")?.textContent).toContain(
      say("shell.error.search"),
    );
  });

  test("a search key the route names is accepted", async () => {
    open("#/rules?q=go");
    await seen(titleOf("rules"));
    expect(window.location.hash).toBe("#/rules?q=go");
  });
});

describe("the chrome", () => {
  test("the footer names the rulebook and the engine", async () => {
    open("#/rules");
    await seen(titleOf("rules"));
    expect(document.querySelector("footer")?.textContent).toBe(
      say("shell.footer", { rulebook: "1.2.0", engine: "0.1.0" }),
    );
  });

  test("an unreleased bundle is marked on every route; a released one on none", async () => {
    open("#/rules");
    await seen(titleOf("rules"));
    expect(document.body.textContent).not.toContain(say("shell.unreleased"));
    root?.unmount();
    router?.history.destroy();
    host?.remove();
    for (const b of blocks.splice(0)) b.remove();

    open(
      "#/rules",
      bundleText.replace('"released": true', '"released": false'),
    );
    for (const [id, path] of nav) {
      await router?.navigate({ to: path });
      await seen(titleOf(id));
      expect(
        `${id}: ${document.body.textContent.includes(say("shell.unreleased"))}`,
      ).toBe(`${id}: true`);
    }
  });
});

describe("the rules view in the shell", () => {
  const count = (): string | null =>
    document.querySelector("[data-part='rules-count']")?.textContent ?? null;
  const total = say("rules.shown", { shown: "15", total: "15" });
  const musts = say("rules.shown", { shown: "6", total: "15" });

  test("a filter in the address narrows the table and reads its own count", async () => {
    open("#/rules?strength=must");
    await seen(titleOf("rules"));
    await expect.poll(count).toBe(musts);
    expect(document.querySelectorAll("tbody tr")).toHaveLength(6);
    expect(window.location.hash).toBe("#/rules?strength=must");
  });

  test("removing a filter chip writes the address and widens the table", async () => {
    open("#/rules?strength=must");
    await expect.poll(count).toBe(musts);
    const remove = document.querySelector<HTMLButtonElement>(
      `button[aria-label="${say("part.filter.remove", { name: say("rules.facet.strength") })}"]`,
    );
    expect(remove === null ? "no remove button" : "found").toBe("found");
    remove?.focus();
    await userEvent.keyboard("{Enter}");
    await expect.poll(count).toBe(total);
    expect(window.location.hash).toBe("#/rules");
  });

  test("clear-all empties the address's search", async () => {
    open("#/rules?strength=must&part=go");
    await seen(titleOf("rules"));
    const clear = [...document.querySelectorAll("button")].find(
      (b) => b.textContent === say("part.filters.clear"),
    );
    expect(clear === undefined ? "no clear button" : "found").toBe("found");
    clear?.focus();
    await userEvent.keyboard("{Enter}");
    await expect.poll(count).toBe(total);
    expect(window.location.hash).toBe("#/rules");
  });

  test("a rule's id opens its page", async () => {
    open("#/rules");
    await expect.poll(count).toBe(total);
    const link = document.querySelector<HTMLAnchorElement>(
      "tbody a[href$='#/rules/go.errors.1']",
    );
    expect(link === null ? "no link" : "found").toBe("found");
    link?.focus();
    await userEvent.keyboard("{Enter}");
    await seen(titleOf("rule"));
    expect(window.location.hash).toBe("#/rules/go.errors.1");
  });
});

describe("the tools view", () => {
  const press = async (label: string): Promise<void> => {
    await expect
      .poll(() =>
        [...document.querySelectorAll("main button")].some(
          (b) => b.textContent === label,
        ),
      )
      .toBe(true);
    const button = [
      ...document.querySelectorAll<HTMLButtonElement>("main button"),
    ].find((b) => b.textContent === label);
    button?.focus();
    await userEvent.keyboard("{Enter}");
  };

  test("the page draws the tools of the bundle under its heading", async () => {
    open("#/tools");
    await seen(titleOf("tools"));
    await expect
      .poll(
        () =>
          document.querySelectorAll("[data-region='tools'] tbody tr").length,
      )
      .toBeGreaterThan(0);
  });

  test("grouping by kind is written to the address and read back from it", async () => {
    open("#/tools");
    await seen(titleOf("tools"));
    await press(say("tools.group.kind"));
    await expect.poll(() => window.location.hash).toContain("group=kind");
    await expect
      .poll(
        () =>
          document
            .querySelector("[data-region='groups']")
            ?.getAttribute("data-group-by") ??
          document
            .querySelector("[data-region='group']")
            ?.getAttribute("data-group"),
      )
      .not.toBeNull();
  });

  test("an address that names a tool opens its panel, and a key nobody knows is refused", async () => {
    open("#/tools?group=kind");
    await seen(titleOf("tools"));
    expect(document.querySelector("[aria-pressed='true']")?.textContent).toBe(
      say("tools.group.kind"),
    );
    root?.unmount();
    router?.history.destroy();
    host?.remove();
    for (const b of blocks.splice(0)) b.remove();
    open("#/tools?bogus=1");
    await seen(say("part.error.heading"));
  });
});

describe("the standing view", () => {
  const toggle = (label: string): HTMLButtonElement | undefined =>
    [...document.querySelectorAll<HTMLButtonElement>("main button")].find(
      (b) => b.textContent === label,
    );
  const pressKey = async (label: string): Promise<void> => {
    await expect.poll(() => toggle(label) !== undefined).toBe(true);
    toggle(label)?.focus();
    await userEvent.keyboard("{Enter}");
  };

  test("draws a bar for each part of the bundle", async () => {
    open("#/standing");
    await seen(titleOf("standing"));
    await expect
      .poll(() => document.querySelectorAll("[data-region='group']").length)
      .toBe(3);
  });

  test("a filter is applied and cleared by keyboard, and survives a reload", async () => {
    open("#/standing");
    await seen(titleOf("standing"));
    await pressKey("go");
    await expect.poll(() => window.location.hash).toContain("part=go");
    await expect
      .poll(() => document.querySelectorAll("[data-region='group']").length)
      .toBe(1);
    const address = window.location.hash;
    root?.unmount();
    router?.history.destroy();
    host?.remove();
    for (const b of blocks.splice(0)) b.remove();
    open(address);
    await seen(titleOf("standing"));
    await expect
      .poll(() => document.querySelectorAll("[data-region='group']").length)
      .toBe(1);
    await pressKey("go");
    await expect.poll(() => window.location.hash).not.toContain("part=");
    await expect
      .poll(() => document.querySelectorAll("[data-region='group']").length)
      .toBe(3);
  });

  test("a key nobody knows is refused", async () => {
    open("#/standing?bogus=1");
    await seen(say("part.error.heading"));
    expect(heading()).toBe(say("part.error.heading"));
  });
});
