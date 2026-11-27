// The coverage view through the shell in a real browser: opened from
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
import { userEvent } from "vitest/browser";
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

const bars = (): number =>
  document.querySelectorAll('main [data-part="coverage-part"]').length;
const current = (): string | null | undefined =>
  document.querySelector('main [aria-current="true"]')?.textContent;

for (const theme of ["light", "dark"] as const) {
  describe(`in ${theme}`, () => {
    test("opens from the hash, applies and clears a standing by keyboard, and survives a reload", async () => {
      document.documentElement.dataset["theme"] = theme;
      open("#/coverage");
      await expect
        .poll(() => document.querySelector("main h1")?.textContent)
        .toBe("Coverage");
      expect(bars()).toBe(3);
      expect(current()).toBe("all standings");

      const link = [
        ...document.querySelectorAll<HTMLAnchorElement>("main nav a"),
      ].find((a) => a.textContent.includes("enforced"));
      link?.focus();
      await userEvent.keyboard("{Enter}");
      await expect
        .poll(() => window.location.hash)
        .toContain("standing=enforced");
      await expect.poll(current).toContain("enforced");

      close();
      open(window.location.hash);
      await expect.poll(bars).toBe(3);
      await expect.poll(current).toContain("enforced");

      const all = document.querySelector<HTMLAnchorElement>("main nav a");
      all?.focus();
      await userEvent.keyboard("{Enter}");
      await expect.poll(() => window.location.hash).toBe("#/coverage");
      await expect.poll(current).toBe("all standings");

      expect(errors).not.toHaveBeenCalled();
      expect(rejections).toEqual([]);
    });
  });
}
