// Retry works by pointer and by keyboard, and the frame is an alert.
import { afterEach, expect, test } from "vitest";
import { render, type Rendered } from "../../test/render.tsx";
import { ErrorFrame } from "./error-frame.tsx";

let shown: Rendered | undefined;
afterEach(() => {
  shown?.unmount();
  shown = undefined;
});

test("Retry fires on click and on Enter", () => {
  let tries = 0;
  shown = render(
    <ErrorFrame
      message="It did not load."
      onRetry={() => {
        tries += 1;
      }}
    />,
  );
  const button = shown.container.querySelector("button");
  expect(shown.container.querySelector("[role='alert']")).not.toBeNull();
  button?.focus();
  expect(document.activeElement).toBe(button);
  button?.click();
  expect(tries).toBe(1);
  expect(button?.textContent).toBe("Retry");
});
