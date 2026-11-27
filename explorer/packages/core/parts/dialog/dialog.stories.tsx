import type { Meta, Story } from "../stories.ts";
import { Dialog, states, type DialogProps } from "./dialog.tsx";

const meta: Meta<DialogProps> = {
  title: "Parts/Dialog",
  states,
  component: Dialog,
  parameters: { a11y: { known: [] } },
};

export default meta;
type S = Story<DialogProps>;

const base = {
  trigger: "Show details",
  title: "Rule details",
  description: "What the rule asks.",
  children: "Wrap an error where it crosses a package.",
};

export const Closed: S = { args: base };
export const Open: S = { args: { ...base, initiallyOpen: true } };
