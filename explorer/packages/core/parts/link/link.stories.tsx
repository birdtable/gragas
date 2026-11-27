import type { Meta, Story } from "../stories.ts";
import { Link, states, type LinkProps } from "./link.tsx";

const meta: Meta<LinkProps> = {
  title: "Parts/Link",
  states,
  component: Link,
  parameters: { a11y: { known: [] } },
};

export default meta;
type S = Story<LinkProps>;

export const Internal: S = { args: { href: "#rules", children: "The rules" } };
export const External: S = {
  args: { href: "https://example.org/", external: true, children: "Example" },
};
