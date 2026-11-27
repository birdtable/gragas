import type { Meta, Story } from "../stories.ts";
import {
  CopyableValue,
  states,
  type CopyableValueProps,
} from "./copyable-value.tsx";

const meta: Meta<CopyableValueProps> = {
  title: "Parts/Copyable value",
  states,
  component: CopyableValue,
  parameters: { a11y: { known: [] } },
};

export default meta;
type S = Story<CopyableValueProps>;

export const Idle: S = { args: { value: "web.parts.17" } };
export const Copied: S = { args: { value: "web.parts.17", copied: true } };
