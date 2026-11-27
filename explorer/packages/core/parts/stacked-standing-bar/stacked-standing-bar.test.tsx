// Segments are drawn in proportion, and the legend says every standing in words
// and symbols with its count, so the bar is never the only account of it.
import { afterEach, expect, test } from "vitest";
import { render, type Rendered } from "../../test/render.tsx";
import { StackedStandingBar } from "./stacked-standing-bar.tsx";

let shown: Rendered | undefined;
afterEach(() => {
  shown?.unmount();
  shown = undefined;
});

test("draws each segment in proportion to its count", () => {
  shown = render(
    <StackedStandingBar
      segments={[
        { standing: "enforced", count: 3 },
        { standing: "planned", count: 1 },
      ]}
    />,
  );
  const widths = [...shown.container.querySelectorAll(".gr-bar-segment")].map(
    (rect) => Number(rect.getAttribute("width")),
  );
  expect(widths).toEqual([75, 25]);
});

test("the legend has a word, a symbol and a count for every standing", () => {
  shown = render(
    <StackedStandingBar
      segments={[
        { standing: "enforced", count: 3 },
        { standing: "planned", count: 0 },
      ]}
    />,
  );
  const items = [...shown.container.querySelectorAll("li")];
  expect(items.map((li) => li.textContent)).toEqual([
    expect.stringContaining("enforced"),
    expect.stringContaining("planned"),
  ]);
  expect(items[0]?.textContent).toContain("3");
  expect(items[1]?.textContent).toContain("0");
  expect(
    shown.container.querySelector("svg")?.getAttribute("aria-hidden"),
  ).toBe("true");
});
