// The status mark never renders a standing without both its word and its
// symbol, with colour taken away (web.accessibility.4), and a mark is drawn in
// a shape of its own as well as its colour.
import { afterEach, describe, expect, test } from "vitest";
import { standings } from "@gragas/tokens";
import { render, themes, type Rendered } from "../../test/render.tsx";
import { StatusMark } from "./status-mark.tsx";

let shown: Rendered | undefined;
afterEach(() => {
  shown?.unmount();
  shown = undefined;
  document.getElementById("no-colour")?.remove();
});

// Every colour the page paints becomes one: what is left is what a person who
// cannot tell colours apart has.
function withoutColour(): void {
  const style = document.createElement("style");
  style.id = "no-colour";
  style.textContent =
    "* { color: #000 !important; background: #fff !important; border-color: #000 !important; }";
  document.head.append(style);
}

describe("a standing", () => {
  for (const { standing, symbol } of standings) {
    test(`${standing} shows its word and its symbol with no colour`, () => {
      withoutColour();
      shown = render(<StatusMark standing={standing} />);
      const part = shown.container.querySelector("[data-part='status-mark']");
      expect(part?.textContent).toContain(standing);
      expect(part?.textContent).toContain(symbol);
      for (const el of part?.querySelectorAll("span") ?? []) {
        if (el.className.includes("gr-hidden")) continue;
        expect(el.getBoundingClientRect().width).toBeGreaterThan(0);
        expect(getComputedStyle(el).visibility).toBe("visible");
      }
    });
  }

  test("takes a different colour role in each theme", () => {
    const colours = themes.map((theme) => {
      shown = render(<StatusMark standing="enforced" />, { theme });
      const part = shown.container.querySelector("[data-part='status-mark']");
      const colour = part === null ? "" : getComputedStyle(part).color;
      shown.unmount();
      shown = undefined;
      return colour;
    });
    expect(colours[0]).not.toBe("");
    expect(colours[0]).not.toBe(colours[1]);
  });
});

describe("a mark", () => {
  test("is a hollow, an outlined and a dotted chip with its word and symbol", () => {
    const seen = (["trial", "blocked", "unproven"] as const).map((mark) => {
      shown = render(<StatusMark standing="planned" mark={mark} />);
      const chip = shown.container.querySelector(`[data-mark='${mark}']`);
      const style = chip === null ? undefined : getComputedStyle(chip);
      const row = {
        mark,
        text: chip?.textContent ?? "",
        style: style?.borderTopStyle ?? "",
        width: style?.borderTopWidth ?? "",
        fill: style?.backgroundColor ?? "",
      };
      shown.unmount();
      shown = undefined;
      return row;
    });
    expect(seen.map((r) => r.text)).toEqual([
      "^trial",
      "!blocked",
      "*unproven",
    ]);
    expect(new Set(seen.map((r) => `${r.style} ${r.width}`)).size).toBe(3);
    // A hollow mark: its fill has no alpha.
    expect(seen[0]?.fill).toMatch(/, 0\)$/);
    expect(seen[2]?.style).toBe("dotted");
  });
});
