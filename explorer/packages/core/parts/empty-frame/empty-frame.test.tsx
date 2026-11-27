// The empty frame shows its title, and its hint only when there is one.
import { afterEach, expect, test } from "vitest";
import { render, type Rendered } from "../../test/render.tsx";
import { EmptyFrame } from "./empty-frame.tsx";

let shown: Rendered | undefined;
afterEach(() => {
  shown?.unmount();
  shown = undefined;
});

test("shows the hint only when given one", () => {
  shown = render(<EmptyFrame title="No findings" />);
  expect(shown.container.querySelector(".gr-frame-hint")).toBeNull();
  shown.rerender(<EmptyFrame title="No findings">Clear a filter.</EmptyFrame>);
  expect(shown.container.querySelector(".gr-frame-hint")?.textContent).toBe(
    "Clear a filter.",
  );
});
