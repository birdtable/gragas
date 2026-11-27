import type { Meta, Story } from "../stories.ts";
import { Count, states, type CountProps } from "./count.tsx";

const meta: Meta<CountProps> = {
  title: "Parts/Count",
  states,
  component: Count,
  parameters: { a11y: { known: [] } },
};

export default meta;
type S = Story<CountProps>;

export const Zero: S = { args: { value: 0 } };
export const One: S = { args: { value: 1 } };
export const Many: S = { args: { value: 42 } };
export const Large: S = { args: { value: 1234567 } };
export const WithUnit: S = { args: { value: 7, unit: "rules" } };
