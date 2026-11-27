// The measure helper (web.parts.17): a rendered view is judged by the
// numbers of its named regions. A region is an element with a data-part
// attribute; the helper reads each one's box, so a spec states a size the way it
// states any other fact, and a part that shrinks below it fails the spec.
export type Measured = Readonly<{
  part: string;
  width: number;
  height: number;
  x: number;
  y: number;
}>;

/** Every region named `part` under `root`, with its box in CSS pixels. */
export function measure(root: ParentNode, part: string): Measured[] {
  return [...root.querySelectorAll(`[data-part~="${part}"]`)].map((el) => {
    const box = el.getBoundingClientRect();
    return { part, width: box.width, height: box.height, x: box.x, y: box.y };
  });
}

/** The smallest width and the smallest height among the measured regions. */
export function smallest(measured: readonly Measured[]): {
  width: number;
  height: number;
} {
  return {
    width: Math.min(...measured.map((m) => m.width)),
    height: Math.min(...measured.map((m) => m.height)),
  };
}

/** The pointer target floor: 24 CSS pixels, 44 where the pointer is coarse. */
export const targetFloor = { fine: 24, coarse: 44 } as const;

const targetSelector =
  "button, a[href], input, select, textarea, [role='tab'], [role='button']";

/** Every pointer target under `root` smaller than `floor` CSS pixels either way. */
export function targetsBelowFloor(
  root: ParentNode,
  floor: number = targetFloor.fine,
): string[] {
  return [...root.querySelectorAll(targetSelector)]
    .filter((el) => {
      const box = el.getBoundingClientRect();
      // A hidden element (visually hidden text, a closed region) is no target.
      return (
        box.width > 1 &&
        box.height > 1 &&
        (box.width < floor || box.height < floor)
      );
    })
    .map((el) => {
      const box = el.getBoundingClientRect();
      return `${el.tagName.toLowerCase()} ${String(Math.round(box.width))}x${String(Math.round(box.height))}`;
    });
}

/** The part names a region carries, in document order, without repeats. */
export function partsIn(root: ParentNode): string[] {
  const names: string[] = [];
  for (const el of root.querySelectorAll("[data-part]")) {
    for (const name of (el.getAttribute("data-part") ?? "").split(/\s+/)) {
      if (name !== "" && !names.includes(name)) names.push(name);
    }
  }
  return names;
}
