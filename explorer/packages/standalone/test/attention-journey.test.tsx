// The attention view through the shell in a real browser: opened from a
// hash URL, in light and in dark, with the fixture bundle; each filter applied and cleared
// by keyboard, in the address, survives a reload; and a console error or an
// unhandled rejection fails it.
import {
  createExplorerClient,
  documentBlocks,
  setClock,
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
let restoreClock: (() => void) | undefined;
beforeEach(() => {
  // The day is pinned, so the count of overdue rules does not move with the real date.
  restoreClock = setClock(() => 1_806_580_800_000);
  errors = vi.spyOn(console, "error");
  window.addEventListener("unhandledrejection", onRejection);
});
afterEach(() => {
  restoreClock?.();
  close();
  window.location.hash = "";
  window.removeEventListener("unhandledrejection", onRejection);
  errors.mockRestore();
  rejections.length = 0;
  delete document.documentElement.dataset["theme"];
});

const listed = (): number =>
  document.querySelectorAll('main [data-part="attention-item"]').length;

const drawn = (): boolean =>
  document.querySelector('main [data-part="attention-filter"]') !== null;

async function press(href: string): Promise<void> {
  const link = document.querySelector<HTMLAnchorElement>(
    `main [data-part="attention-filter"] a[href="${href}"]`,
  );
  link?.focus();
  expect(document.activeElement).toBe(link);
  await userEvent.keyboard("{Enter}");
}

for (const theme of ["light", "dark"] as const) {
  describe(`in ${theme}`, () => {
    test("opens from the hash, shows the eight kinds and the day, and survives a reload", async () => {
      document.documentElement.dataset["theme"] = theme;
      open("#/attention");
      await expect
        .poll(() => document.querySelector("main h1")?.textContent)
        .toBe("Attention");
      expect(listed()).toBe(32);
      expect(
        [
          ...document.querySelectorAll('main [data-part="attention-group"]'),
        ].map((g) => g.getAttribute("data-kind")),
      ).toHaveLength(8);
      expect(
        document.querySelector('main [data-part="attention-today"]')
          ?.textContent,
      ).toContain("1 Apr 2027");
      expect(
        document.querySelector('main [data-part="attention-override-rate"]')
          ?.textContent,
      ).toBe("override rate: no batches given");

      close();
      open(window.location.hash);
      await expect.poll(listed).toBe(32);
      expect(window.location.hash).toBe("#/attention");

      expect(errors).not.toHaveBeenCalled();
      expect(rejections).toEqual([]);
    });

    test("applies and clears each kind by keyboard, and a reload finds the same state", async () => {
      document.documentElement.dataset["theme"] = theme;
      open("#/attention");
      await expect.poll(listed).toBe(32);

      for (const [kind, count] of [
        ["blocked", 2],
        ["overdue", 11],
        ["unproven", 8],
        ["trial-overrun", 0],
        ["override-rate", 0],
      ] as const) {
        await press(`#/attention?kind=${kind}`);
        await expect
          .poll(() => window.location.hash)
          .toBe(`#/attention?kind=${kind}`);
        await expect.poll(listed).toBe(count);

        close();
        open(window.location.hash);
        await expect.poll(drawn).toBe(true);
        await expect.poll(listed).toBe(count);
        expect(
          document.querySelector('main [aria-current="true"]')?.textContent,
        ).not.toBe("all");

        await press("#/attention");
        await expect.poll(() => window.location.hash).toBe("#/attention");
        await expect.poll(listed).toBe(32);
      }

      expect(errors).not.toHaveBeenCalled();
      expect(rejections).toEqual([]);
    });
  });
}
