// By keyboard: typing narrows the commands, Enter runs the first match and
// closes, and no match says so.
import { userEvent } from "vitest/browser";
import { afterEach, expect, test } from "vitest";
import { render, type Rendered } from "../../test/render.tsx";
import { CommandPalette } from "./command-palette.tsx";

let shown: Rendered | undefined;
afterEach(() => {
  shown?.unmount();
  shown = undefined;
});

const commands = [
  { id: "rules", label: "Go to rules" },
  { id: "theme", label: "Switch theme" },
];
const popup = (): HTMLElement | null =>
  document.querySelector<HTMLElement>(
    "[data-part='command-palette'][data-region='popup']",
  );

test("typing narrows the commands and Enter runs the first match", async () => {
  const ran: string[] = [];
  shown = render(
    <CommandPalette
      trigger="Commands"
      commands={commands}
      onRun={(id) => ran.push(id)}
      initiallyOpen
    />,
  );
  await shown.settled();
  popup()?.querySelector("input")?.focus();
  await userEvent.keyboard("theme{Enter}");
  await shown.settled();
  expect(ran).toEqual(["theme"]);
  expect(popup()).toBeNull();
});

test("a query with no match says so", async () => {
  shown = render(
    <CommandPalette
      trigger="Commands"
      commands={commands}
      onRun={() => undefined}
      initiallyOpen
      initialQuery="zzz"
    />,
  );
  await shown.settled();
  expect(popup()?.querySelector("[role='status']")?.textContent).toBe(
    "No matching command",
  );
  expect(popup()?.querySelectorAll(".gr-palette-command")).toHaveLength(0);
});
