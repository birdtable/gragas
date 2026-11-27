import type { Meta, Story } from "../stories.ts";
import { EmptyFrame, states, type EmptyFrameProps } from "./empty-frame.tsx";

const meta: Meta<EmptyFrameProps> = {
  title: "Parts/Empty frame",
  states,
  component: EmptyFrame,
  parameters: { a11y: { known: [] } },
};

export default meta;
type S = Story<EmptyFrameProps>;

export const TitleOnly: S = { args: { title: "No findings" } };
export const WithHint: S = {
  args: { title: "No findings", children: "Clear a filter to see more." },
};
