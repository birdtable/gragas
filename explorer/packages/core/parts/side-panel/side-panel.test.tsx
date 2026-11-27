// The panel is a named region, closes by its button, and never outgrows the
// dynamic viewport.
import { userEvent } from "vitest/browser";
import { afterEach, expect, test } from "vitest";
import { render, type Rendered } from "../../test/render.tsx";
import { SidePanel } from "./side-panel.tsx";

let shown: Rendered | undefined;
afterEach(() => {
  shown?.unmount();
  shown = undefined;
});

test("is a region named by its title that closes by its button", () => {
  let closed = 0;
  shown = render(
    <SidePanel
      title="go.errors.1"
      onClose={() => {
        closed += 1;
      }}
    >
      Body
    </SidePanel>,
  );
  const aside = shown.container.querySelector("aside");
  expect(aside?.getAttribute("aria-label")).toBe("go.errors.1");
  const button = shown.container.querySelector("button");
  expect(button?.getAttribute("aria-label")).toBe("Close panel");
  button?.click();
  expect(closed).toBe(1);
});

test("is no taller than the dynamic viewport", () => {
  shown = render(
    <SidePanel title="t" onClose={() => undefined}>
      {"long ".repeat(4000)}
    </SidePanel>,
  );
  const box = shown.container.querySelector("aside")?.getBoundingClientRect();
  expect(box?.height).toBeLessThanOrEqual(window.innerHeight);
});

test("closes by Enter and by Space on its button", async () => {
  let closed = 0;
  shown = render(
    <SidePanel
      title="go.errors.1"
      onClose={() => {
        closed += 1;
      }}
    >
      Body
    </SidePanel>,
  );
  shown.container.querySelector("button")?.focus();
  await userEvent.keyboard("{Enter}");
  expect(closed).toBe(1);
  await userEvent.keyboard(" ");
  expect(closed).toBe(2);
});
