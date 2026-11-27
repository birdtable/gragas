import type { Meta, Story } from "../stories.ts";
import { Banner, states, type BannerProps } from "./banner.tsx";

const meta: Meta<BannerProps> = {
  title: "Parts/Banner",
  states,
  component: Banner,
  parameters: { a11y: { known: [] } },
};

export default meta;
type S = Story<BannerProps>;

export const Info: S = {
  args: { tone: "info", children: "A new rulebook is out." },
};
export const Success: S = { args: { tone: "success", children: "Saved." } };
export const Warning: S = {
  args: { tone: "warning", children: "Two rules have no proof." },
};
export const Error: S = {
  args: { tone: "error", children: "The run did not finish." },
};
export const Dismissible: S = {
  args: { tone: "info", children: "Read me.", onDismiss: () => undefined },
};
