// The dialog's contract: focus goes in, stays in, comes back on close, and
// Escape closes only the dialog on top.
import { userEvent } from "vitest/browser";
import { afterEach, expect, test } from "vitest";
import { render, type Rendered } from "../../test/render.tsx";
import { Dialog } from "./dialog.tsx";

let shown: Rendered | undefined;
afterEach(() => {
  shown?.unmount();
  shown = undefined;
});

const popups = "[data-part='dialog'][data-region='popup']";
const popup = (): HTMLElement | null =>
  document.querySelector<HTMLElement>(popups);

async function opened(
  ui = (
    <Dialog trigger="Open" title="Title">
      Body
    </Dialog>
  ),
  press = true,
): Promise<HTMLButtonElement | null> {
  shown = render(ui);
  const trigger = shown.container.querySelector("button");
  if (press) {
    trigger?.focus();
    await userEvent.keyboard("{Enter}");
  }
  await shown.settled();
  return trigger;
}

test("focus moves inside when it opens", async () => {
  await opened();
  expect(popup()).not.toBeNull();
  expect(popup()?.contains(document.activeElement)).toBe(true);
});

test("focus is trapped: Tab never leaves the dialog", async () => {
  await opened();
  for (let i = 0; i < 6; i += 1) {
    await userEvent.keyboard("{Tab}");
    // The library moves focus back from its edge guard a frame later.
    await shown?.settled();
    expect(popup()?.contains(document.activeElement)).toBe(true);
  }
});

test("Escape closes it and focus returns to the trigger", async () => {
  const trigger = await opened();
  await userEvent.keyboard("{Escape}");
  await shown?.settled();
  expect(popup()).toBeNull();
  expect(document.activeElement).toBe(trigger);
});

test("the Close button closes it", async () => {
  const trigger = await opened();
  const close = popup()?.querySelector("button");
  close?.click();
  await shown?.settled();
  expect(popup()).toBeNull();
  expect(document.activeElement).toBe(trigger);
});

test("Escape closes only the dialog on top", async () => {
  await opened(
    <Dialog trigger="Open" title="Outer" initiallyOpen>
      <Dialog trigger="Inner" title="Inner title">
        Inner body
      </Dialog>
    </Dialog>,
    false,
  );
  const inner = [...(popup()?.querySelectorAll("button") ?? [])].find(
    (b) => b.textContent === "Inner",
  );
  inner?.focus();
  await userEvent.keyboard("{Enter}");
  await shown?.settled();
  expect(document.querySelectorAll(popups)).toHaveLength(2);
  await userEvent.keyboard("{Escape}");
  await shown?.settled();
  expect(document.querySelectorAll(popups)).toHaveLength(1);
  expect(popup()?.textContent).toContain("Outer");
});
