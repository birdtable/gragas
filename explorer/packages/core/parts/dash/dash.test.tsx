// A dash always carries its reason, in the words a reader hears and the title.
import { afterEach, expect, test } from "vitest";
import { render, type Rendered } from "../../test/render.tsx";
import { Dash } from "./dash.tsx";

let shown: Rendered | undefined;
afterEach(() => {
  shown?.unmount();
  shown = undefined;
});

test("says none, with the reason", () => {
  shown = render(<Dash reason="not measured yet" />);
  const part = shown.container.querySelector("[data-part='dash']");
  expect(part?.textContent).toContain("none, not measured yet");
  expect(part?.getAttribute("title")).toBe("not measured yet");
});
