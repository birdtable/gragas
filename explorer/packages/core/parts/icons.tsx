// The one icon registry: each icon is a path on a 16 by 16 grid,
// drawn in the text colour, hidden from assistive technology because a part
// that shows one also carries a word. A part names an icon; none draws its own.
import type { ReactElement } from "react";

export const icons = {
  check: "M3 8.5l3 3 7-7",
  chevron: "M4 6l4 4 4-4",
  close: "M4 4l8 8M12 4l-8 8",
  copy: "M5 5V2.5h8.5V11H11M2.5 5H11v8.5H2.5z",
  info: "M8 7v5M8 4.2v.1",
  search: "M7 12a5 5 0 1 1 0-10 5 5 0 0 1 0 10zM11 11l3.5 3.5",
  warning: "M8 2.5l6 11H2zM8 7v3M8 11.8v.1",
} as const;

export type IconName = keyof typeof icons;

export function Icon({ name }: { readonly name: IconName }): ReactElement {
  return (
    <svg
      data-icon={name}
      width="16"
      height="16"
      viewBox="0 0 16 16"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.5"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      focusable="false"
    >
      <path d={icons[name]} />
    </svg>
  );
}
