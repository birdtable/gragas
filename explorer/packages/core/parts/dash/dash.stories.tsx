import type { Meta, Story } from "../stories.ts";
import { Dash, states, type DashProps } from "./dash.tsx";

const meta: Meta<DashProps> = {
  title: "Parts/Dash with reason",
  states,
  component: Dash,
  parameters: { a11y: { known: [] } },
};

export default meta;
type S = Story<DashProps>;

export const NotApplicable: S = { args: { reason: "not applicable" } };
export const NotMeasured: S = { args: { reason: "not measured yet" } };
