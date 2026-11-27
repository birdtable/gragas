import type { Meta, Story } from "../stories.ts";
import { FilterChip, states, type FilterChipProps } from "./filter-chip.tsx";

const meta: Meta<FilterChipProps> = {
  title: "Parts/Filter chip",
  states,
  component: FilterChip,
  parameters: { a11y: { known: [] } },
};

export default meta;
type S = Story<FilterChipProps>;

export const Applied: S = {
  args: { name: "standing", value: "enforced", onRemove: () => undefined },
};
