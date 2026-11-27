import type { Meta, Story } from "../stories.ts";
import {
  StackedStandingBar,
  states,
  type StackedStandingBarProps,
} from "./stacked-standing-bar.tsx";

const meta: Meta<StackedStandingBarProps> = {
  title: "Parts/Stacked standing bar",
  states,
  component: StackedStandingBar,
  parameters: { a11y: { known: [] } },
};

export default meta;
type S = Story<StackedStandingBarProps>;

export const Mixed: S = {
  args: {
    segments: [
      { standing: "enforced", count: 12 },
      { standing: "reviewed", count: 5 },
      { standing: "planned", count: 3 },
      { standing: "proposed", count: 2 },
    ],
  },
};
export const Single: S = {
  args: { segments: [{ standing: "enforced", count: 9 }] },
};
export const None: S = {
  args: {
    segments: [
      { standing: "enforced", count: 0 },
      { standing: "planned", count: 0 },
    ],
  },
};
