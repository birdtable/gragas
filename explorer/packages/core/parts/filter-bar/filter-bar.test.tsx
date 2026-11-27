// Chips remove one filter by name; Clear removes all and is absent with none.
import { userEvent } from "vitest/browser";
import { afterEach, expect, test } from "vitest";
import { render, type Rendered } from "../../test/render.tsx";
import { FilterBar } from "./filter-bar.tsx";

let shown: Rendered | undefined;
afterEach(() => {
  shown?.unmount();
  shown = undefined;
});

test("removes the filter whose chip is pressed, and clears all", () => {
  const removed: string[] = [];
  let cleared = 0;
  shown = render(
    <FilterBar
      filters={[
        { name: "standing", value: "enforced" },
        { name: "part", value: "go" },
      ]}
      onRemove={(name) => removed.push(name)}
      onClear={() => {
        cleared += 1;
      }}
    />,
  );
  const buttons = [...shown.container.querySelectorAll("button")];
  buttons[1]?.click();
  buttons[2]?.click();
  expect(removed).toEqual(["part"]);
  expect(cleared).toBe(1);
});

test("shows no Clear button with no filters", () => {
  shown = render(
    <FilterBar
      filters={[]}
      onRemove={() => undefined}
      onClear={() => undefined}
    />,
  );
  expect(shown.container.querySelector("button")).toBeNull();
});

test("removes a filter and clears all by Enter and by Space", async () => {
  const removed: string[] = [];
  let cleared = 0;
  shown = render(
    <FilterBar
      filters={[
        { name: "standing", value: "enforced" },
        { name: "part", value: "go" },
      ]}
      onRemove={(name) => removed.push(name)}
      onClear={() => {
        cleared += 1;
      }}
    />,
  );
  const buttons = [...shown.container.querySelectorAll("button")];
  buttons[0]?.focus();
  await userEvent.keyboard("{Enter}");
  buttons[1]?.focus();
  await userEvent.keyboard(" ");
  buttons[2]?.focus();
  await userEvent.keyboard("{Enter}");
  await userEvent.keyboard(" ");
  expect(removed).toEqual(["standing", "part"]);
  expect(cleared).toBe(2);
});
