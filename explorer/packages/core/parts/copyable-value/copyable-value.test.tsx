// The copied word and the live region arrive together, and the button's name
// carries the visible word (label in name).
import { userEvent } from "vitest/browser";
import { afterEach, expect, test, vi } from "vitest";
import { render, type Rendered } from "../../test/render.tsx";
import { CopyableValue } from "./copyable-value.tsx";

let shown: Rendered | undefined;
afterEach(() => {
  shown?.unmount();
  shown = undefined;
});

test("the copied state says Copied on the button and in the live region", () => {
  shown = render(<CopyableValue value="web.parts.17" copied />);
  expect(shown.container.querySelector("button")?.textContent).toBe("Copied");
  expect(shown.container.querySelector("[role='status']")?.textContent).toBe(
    "Copied",
  );
});

test("the button's name holds its visible word and names the value", () => {
  shown = render(<CopyableValue value="web.parts.17" />);
  const button = shown.container.querySelector("button");
  expect(button?.getAttribute("aria-label")).toBe("Copy web.parts.17");
  expect(button?.textContent).toBe("Copy");
});

test("copies by Enter and by Space, and then says Copied", async () => {
  // A headless page may not write the clipboard; the part's answer is the test.
  const write = vi
    .spyOn(navigator.clipboard, "writeText")
    .mockResolvedValue(undefined);
  shown = render(<CopyableValue value="web.parts.17" />);
  const button = shown.container.querySelector("button");
  button?.focus();
  await userEvent.keyboard("{Enter}");
  await expect
    .poll(() => shown?.container.querySelector("[role='status']")?.textContent)
    .toBe("Copied");
  shown.unmount();
  shown = render(<CopyableValue value="web.parts.17" />);
  shown.container.querySelector("button")?.focus();
  await userEvent.keyboard(" ");
  await expect
    .poll(() => shown?.container.querySelector("button")?.textContent)
    .toBe("Copied");
  expect(write).toHaveBeenCalledTimes(2);
  expect(write).toHaveBeenCalledWith("web.parts.17");
  write.mockRestore();
});
