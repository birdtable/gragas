// A tile reads label first, then value, then the note.
import { afterEach, expect, test } from "vitest";
import { render, type Rendered } from "../../test/render.tsx";
import { Tile } from "./tile.tsx";

let shown: Rendered | undefined;
afterEach(() => {
  shown?.unmount();
  shown = undefined;
});

test("is a term, its value and its note, in that order", () => {
  shown = render(<Tile label="Rules" value="128" note="12 proposed" />);
  const kids = [...(shown.container.querySelector("dl")?.children ?? [])];
  expect(kids.map((el) => el.tagName)).toEqual(["DT", "DD", "DD"]);
  expect(kids.map((el) => el.textContent)).toEqual([
    "Rules",
    "128",
    "12 proposed",
  ]);
});
