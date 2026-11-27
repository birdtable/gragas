import type { Meta, Story } from "../stories.ts";
import { SidePanel, states, type SidePanelProps } from "./side-panel.tsx";

const meta: Meta<SidePanelProps> = {
  title: "Parts/Side panel",
  states,
  component: SidePanel,
  parameters: { a11y: { known: [] } },
};

export default meta;
type S = Story<SidePanelProps>;

export const Short: S = {
  args: {
    title: "go.errors.1",
    onClose: () => undefined,
    children: "Wrap errors.",
  },
};
export const Long: S = {
  args: {
    title: "go.errors.1",
    onClose: () => undefined,
    children: "Wrap an error with context where it crosses a package. ".repeat(
      12,
    ),
  },
};
