// Headers are real header cells; no rows is the empty frame, not a bare header.
import { afterEach, expect, test } from "vitest";
import { render, type Rendered } from "../../test/render.tsx";
import { ListTable } from "./list-table.tsx";

let shown: Rendered | undefined;
afterEach(() => {
  shown?.unmount();
  shown = undefined;
});

const columns = [
  { key: "rule", label: "Rule" },
  { key: "n", label: "Count", numeric: true },
] as const;

test("draws a header cell per column and a row per row", () => {
  shown = render(
    <ListTable
      caption="Rules"
      columns={columns}
      rows={[
        { id: "a", cells: { rule: "a", n: "1" } },
        { id: "b", cells: { rule: "b", n: "2" } },
      ]}
    />,
  );
  expect(shown.container.querySelectorAll("th[scope='col']")).toHaveLength(2);
  expect(shown.container.querySelectorAll("tbody tr")).toHaveLength(2);
  expect(shown.container.querySelector("caption")?.textContent).toBe("Rules");
});

test("with no rows shows the empty frame and no table", () => {
  shown = render(<ListTable caption="Rules" columns={columns} rows={[]} />);
  expect(shown.container.querySelector("table")).toBeNull();
  expect(
    shown.container.querySelector("[data-part='empty-frame']"),
  ).not.toBeNull();
});
