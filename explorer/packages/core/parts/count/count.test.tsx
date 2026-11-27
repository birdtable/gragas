// The count says its number as the formatter does, never as the engine's default.
import { afterEach, expect, test } from "vitest";
import { render, type Rendered } from "../../test/render.tsx";
import { Count } from "./count.tsx";

let shown: Rendered | undefined;
afterEach(() => {
  shown?.unmount();
  shown = undefined;
});

test("groups a large number through the formatter", () => {
  shown = render(<Count value={1234567} />);
  expect(shown.container.textContent).toBe("1,234,567");
});

test("puts the unit beside the number", () => {
  shown = render(<Count value={7} unit="rules" />);
  expect(shown.container.textContent).toBe("7 rules");
});
