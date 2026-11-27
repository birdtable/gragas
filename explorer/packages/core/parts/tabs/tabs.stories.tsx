import type { Meta, Story } from "../stories.ts";
import { Tabs, states, type TabsProps } from "./tabs.tsx";

const meta: Meta<TabsProps> = {
  title: "Parts/Tabs",
  states,
  component: Tabs,
  parameters: { a11y: { known: [] } },
};

export default meta;
type S = Story<TabsProps>;

const tabs = [
  { id: "rules", label: "Rules", panel: "The rules." },
  { id: "tools", label: "Tools", panel: "The tools." },
  { id: "proof", label: "Proof", panel: "The proof." },
] as const;

export const FirstSelected: S = { args: { label: "Views", tabs } };
export const SecondSelected: S = {
  args: { label: "Views", tabs, selected: "tools" },
};
