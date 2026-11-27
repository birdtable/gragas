import type { Meta, Story } from "../stories.ts";
import { ErrorFrame, states, type ErrorFrameProps } from "./error-frame.tsx";

const meta: Meta<ErrorFrameProps> = {
  title: "Parts/Error frame",
  states,
  component: ErrorFrame,
  parameters: { a11y: { known: [] } },
};

export default meta;
type S = Story<ErrorFrameProps>;

export const WithRetry: S = {
  args: {
    message: "The rulebook did not load.",
    onRetry: () => undefined,
  },
};
