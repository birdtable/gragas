// The health view through the shell in a real browser: opened from
// a hash URL, in light and in dark, with the fixture bundle; a reload finds the
// same page; and a console error or an unhandled rejection fails it.
import {
  createExplorerClient,
  documentBlocks,
  type DataSource,
} from "@gragas/core";
import { RouterProvider } from "@tanstack/react-router";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import bundleText from "../../../../internal/host/assets/contract/fixture/bundle.json?raw";
import { createAppRouter, type AppRouter } from "../src/router.tsx";

let root: Root | undefined;
let router: AppRouter | undefined;
let host: HTMLElement | undefined;
let block: HTMLScriptElement | undefined;
const rejections: unknown[] = [];
const onRejection = (e: PromiseRejectionEvent): void => {
  rejections.push(e.reason);
};

function open(hash: string): void {
  block = document.createElement("script");
  block.type = "application/json";
  block.dataset["file"] = "bundle.json";
  block.textContent = bundleText;
  document.head.append(block);
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

function close(): void {
  root?.unmount();
  router?.history.destroy();
  host?.remove();
  block?.remove();
  root = undefined;
  router = undefined;
  host = undefined;
  block = undefined;
}

let errors: ReturnType<typeof vi.spyOn>;
beforeEach(() => {
  errors = vi.spyOn(console, "error");
  window.addEventListener("unhandledrejection", onRejection);
});
afterEach(() => {
  close();
  window.location.hash = "";
  window.removeEventListener("unhandledrejection", onRejection);
  errors.mockRestore();
  rejections.length = 0;
  delete document.documentElement.dataset["theme"];
});

const tiles = (): number =>
  document.querySelectorAll('main [data-part="health-tile"]').length;

for (const theme of ["light", "dark"] as const) {
  describe(`in ${theme}`, () => {
    test("opens from the hash, shows seven tiles under two labelled sets, and survives a reload", async () => {
      document.documentElement.dataset["theme"] = theme;
      open("#/health");
      await expect
        .poll(() => document.querySelector("main h1")?.textContent)
        .toBe("Health");
      expect(tiles()).toBe(7);
      expect(
        [...document.querySelectorAll("main h2")].map((h) => h.textContent),
      ).toContain("Its own rules");

      close();
      open(window.location.hash);
      await expect.poll(tiles).toBe(7);
      expect(window.location.hash).toBe("#/health");

      expect(errors).not.toHaveBeenCalled();
      expect(rejections).toEqual([]);
    });
  });
}

test("the footer and the page agree on the release", async () => {
  open("#/health");
  await expect.poll(tiles).toBe(7);
  expect(document.querySelector("main")?.textContent).toContain("1.2.0");
});
