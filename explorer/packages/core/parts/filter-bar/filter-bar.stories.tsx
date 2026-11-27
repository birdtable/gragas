import type { Meta, Story } from "../stories.ts";
import { FilterBar, states, type FilterBarProps } from "./filter-bar.tsx";

const meta: Meta<FilterBarProps> = {
  title: "Parts/Filter bar",
  states,
  component: FilterBar,
  parameters: { a11y: { known: [] } },
};

export default meta;
type S = Story<FilterBarProps>;

const none = { onRemove: () => undefined, onClear: () => undefined };

export const None: S = { args: { filters: [], ...none } };
export const One: S = {
  args: { filters: [{ name: "standing", value: "enforced" }], ...none },
};
export const Several: S = {
  args: {
    filters: [
      { name: "standing", value: "enforced" },
      { name: "part", value: "go" },
      { name: "tool", value: "golangci-lint" },
    ],
    ...none,
  },
};
