// // The changelog view through the shell in a real browser: opened from a
// hash URL, in light and in dark, with the fixture bundle; each filter applied and cleared
// by keyboard, in the address, survives a reload; and a console error or an
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
  document.querySelectorAll('main [data-part="changelog-change"]').length;

async function press(href: string): Promise<void> {
  const link = document.querySelector<HTMLAnchorElement>(
    `main [data-part="changelog-filter"] a[href="${href}"]`,
  );
  link?.focus();
  expect(document.activeElement).toBe(link);
  await userEvent.keyboard("{Enter}");
}

for (const theme of ["light", "dark"] as const) {
  describe(`in ${theme}`, () => {
    test("opens from the hash, lists the releases newest first, and survives a reload", async () => {
      document.documentElement.dataset["theme"] = theme;
      open("#/changelog");
      await expect
        .poll(() => document.querySelector("main h1")?.textContent)
        .toBe("Changelog");
      expect(listed()).toBe(8);
      expect(
        [
          ...document.querySelectorAll(
            'main [data-part="changelog-release"] h2',
          ),
        ].map((h) => h.textContent),
      ).toEqual(["Release 1.2.0", "Release 1.1.0", "Release 1.0.0"]);

      close();
      open(window.location.hash);
      await expect.poll(listed).toBe(8);
      expect(window.location.hash).toBe("#/changelog");

      expect(errors).not.toHaveBeenCalled();
      expect(rejections).toEqual([]);
    });

    test("applies and clears each filter by keyboard, and a reload finds the same state", async () => {
      document.documentElement.dataset["theme"] = theme;
      open("#/changelog");
      await expect.poll(listed).toBe(8);

      await press("#/changelog?change=replaced");
      await expect
        .poll(() => window.location.hash)
        .toBe("#/changelog?change=replaced");
      await expect.poll(listed).toBe(1);

      close();
      open(window.location.hash);
      await expect.poll(listed).toBe(1);

      await press("#/changelog?change=replaced&action=none");
      await expect
        .poll(() => window.location.hash)
        .toBe("#/changelog?change=replaced&action=none");
      await expect.poll(listed).toBe(0);

      await press("#/changelog?change=replaced");
      await expect.poll(listed).toBe(1);

      await press("#/changelog");
      await expect.poll(() => window.location.hash).toBe("#/changelog");
      await expect.poll(listed).toBe(8);

      expect(errors).not.toHaveBeenCalled();
      expect(rejections).toEqual([]);
    });
  });
}
