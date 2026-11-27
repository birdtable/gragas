// By keyboard: the arrow keys move between tabs, and the selected tab's panel is
// the one shown.
import { userEvent } from "vitest/browser";
import { afterEach, expect, test } from "vitest";
import { render, type Rendered } from "../../test/render.tsx";
import { Tabs } from "./tabs.tsx";

let shown: Rendered | undefined;
afterEach(() => {
  shown?.unmount();
  shown = undefined;
});

const tabs = [
  { id: "rules", label: "Rules", panel: "The rules." },
  { id: "tools", label: "Tools", panel: "The tools." },
] as const;

test("ArrowRight moves focus to the next tab", async () => {
  shown = render(<Tabs label="Views" tabs={tabs} />);
  await shown.settled();
  const [first, second] = [...shown.container.querySelectorAll("[role='tab']")];
  expect(first?.getAttribute("aria-selected")).toBe("true");
  if (first instanceof HTMLElement) first.focus();
  await userEvent.keyboard("{ArrowRight}");
  expect(document.activeElement).toBe(second);
});

test("shows the panel of the selected tab", async () => {
  shown = render(<Tabs label="Views" tabs={tabs} selected="tools" />);
  await shown.settled();
  expect(shown.container.querySelector("[role='tabpanel']")?.textContent).toBe(
    "The tools.",
  );
});
