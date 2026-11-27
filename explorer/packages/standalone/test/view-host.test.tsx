// The host in a real browser: a view's onSearch updates the
// address, and a value the schema refuses leaves it alone. The page is a stub
// that calls onSearch from two buttons; the router, the route tree and the
// address are the shell's own.
import {
  createExplorerClient,
  documentBlocks,
  type DataSource,
} from "@gragas/core";
import {
  RouterContextProvider,
  createMemoryHistory,
} from "@tanstack/react-router";
import type { ReactElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { page, userEvent } from "vitest/browser";
import { afterEach, expect, test } from "vitest";
import * as z from "zod";
import { defineView, type PageProps } from "../src/manifest.ts";
import { createAppRouter, type AppRouter } from "../src/router.tsx";
import { ViewHost } from "../src/view-host.tsx";

// An empty q is a value the schema refuses; the route's own schema takes q.
const searchSchema = z.strictObject({ q: z.string().min(1).optional() });

function Page({ onSearch }: PageProps<typeof searchSchema>): ReactElement {
  return (
    <div>
      <button
        type="button"
        onClick={() => {
          onSearch?.({ q: "wrap" });
        }}
      >
        good
      </button>
      <button
        type="button"
        onClick={() => {
          onSearch?.({ q: "" });
        }}
      >
        bad
      </button>
    </div>
  );
}

const manifest = defineView({ search: searchSchema, Page });

let root: Root | undefined;
let router: AppRouter | undefined;
let host: HTMLElement | undefined;

afterEach(() => {
  root?.unmount();
  router?.history.destroy();
  host?.remove();
  root = undefined;
  router = undefined;
  host = undefined;
});

async function mount(): Promise<AppRouter> {
  const source: DataSource = {
    base: "http://127.0.0.1:9/",
    inline: documentBlocks(document),
  };
  const made = createAppRouter({
    client: createExplorerClient(),
    source,
    history: createMemoryHistory({ initialEntries: ["/rules"] }),
  });
  await made.load();
  router = made;
  host = document.createElement("div");
  document.body.append(host);
  root = createRoot(host);
  root.render(
    <RouterContextProvider router={made}>
      <ViewHost manifest={manifest} search={{}} params={{}} />
    </RouterContextProvider>,
  );
  return made;
}

const searchOf = (r: AppRouter): string => r.history.location.search;

test("an accepted search is written to the address", async () => {
  const made = await mount();
  await userEvent.click(page.getByRole("button", { name: "good" }));
  await expect.poll(() => searchOf(made)).toBe("?q=wrap");
});

test("a search the schema refuses leaves the address alone", async () => {
  const made = await mount();
  await userEvent.click(page.getByRole("button", { name: "bad" }));
  await userEvent.click(page.getByRole("button", { name: "good" }));
  await expect.poll(() => searchOf(made)).toBe("?q=wrap");
  expect(made.history.location.pathname).toBe("/rules");
});
