// The remove button names its filter and removes it by click.
import { userEvent } from "vitest/browser";
import { afterEach, expect, test } from "vitest";
import { render, type Rendered } from "../../test/render.tsx";
import { FilterChip } from "./filter-chip.tsx";

let shown: Rendered | undefined;
afterEach(() => {
  shown?.unmount();
  shown = undefined;
});

test("removes itself and says which filter it removes", () => {
  let removed = 0;
  shown = render(
    <FilterChip
      name="standing"
      value="enforced"
      onRemove={() => {
        removed += 1;
      }}
    />,
  );
  const button = shown.container.querySelector("button");
  expect(button?.getAttribute("aria-label")).toBe("Remove filter standing");
  button?.click();
  expect(removed).toBe(1);
});

test("removes itself by Enter and by Space", async () => {
  let removed = 0;
  shown = render(
    <FilterChip
      name="standing"
      value="enforced"
      onRemove={() => {
        removed += 1;
      }}
    />,
  );
  shown.container.querySelector("button")?.focus();
  await userEvent.keyboard("{Enter}");
  expect(removed).toBe(1);
  await userEvent.keyboard(" ");
  expect(removed).toBe(2);
});
