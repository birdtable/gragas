// // The terms view through the shell in a real browser: opened from a
// hash URL, in light and in dark, with the fixture bundle; the language chosen
// in the address, by keyboard, survives a reload; and a console error or an
// unhandled rejection fails it.
import {
  createExplorerClient,
  documentBlocks,
  type DataSource,
} from "@gragas/core";
import { userEvent } from "vitest/browser";
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

const listed = (): number =>
  document.querySelectorAll('main [data-part="terms-term"]').length;

for (const theme of ["light", "dark"] as const) {
  describe(`in ${theme}`, () => {
    test("opens from the hash, lists the terms, and survives a reload", async () => {
      document.documentElement.dataset["theme"] = theme;
      open("#/terms");
      await expect
        .poll(() => document.querySelector("main h1")?.textContent)
        .toBe("Terms");
      expect(listed()).toBe(2);

      close();
      open(window.location.hash);
      await expect.poll(listed).toBe(2);
      expect(window.location.hash).toBe("#/terms");

      expect(errors).not.toHaveBeenCalled();
      expect(rejections).toEqual([]);
    });

    test("chooses French by keyboard, shows the stale mark, and a reload finds the same state", async () => {
      document.documentElement.dataset["theme"] = theme;
      open("#/terms");
      await expect.poll(listed).toBe(2);
      const french = document.querySelector<HTMLAnchorElement>(
        'main [data-part="terms-switch"] a[href="#/terms?lang=fr"]',
      );
      french?.focus();
      expect(document.activeElement).toBe(french);
      await userEvent.keyboard("{Enter}");
      await expect.poll(() => window.location.hash).toBe("#/terms?lang=fr");
      await expect
        .poll(() => document.querySelector("main #term-wrap")?.textContent)
        .toContain("stale");

      close();
      open(window.location.hash);
      await expect.poll(listed).toBe(2);
      expect(document.querySelector("main #term-gate")?.textContent).toContain(
        "Un contrôle qui arrête",
      );

      // Back to the authoritative language by keyboard clears the choice.
      document
        .querySelector<HTMLAnchorElement>(
          'main [data-part="terms-switch"] a[href="#/terms"]',
        )
        ?.focus();
      await userEvent.keyboard("{Enter}");
      await expect.poll(() => window.location.hash).toBe("#/terms");

      expect(errors).not.toHaveBeenCalled();
      expect(rejections).toEqual([]);
    });
  });
}
