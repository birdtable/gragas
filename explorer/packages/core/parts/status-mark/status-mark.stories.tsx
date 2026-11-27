import type { Meta, Story } from "../stories.ts";
import { StatusMark, states, type StatusMarkProps } from "./status-mark.tsx";

const meta: Meta<StatusMarkProps> = {
  title: "Parts/Status mark",
  states,
  component: StatusMark,
  parameters: { a11y: { known: [] } },
};

export default meta;
type S = Story<StatusMarkProps>;

export const Proposed: S = { args: { standing: "proposed" } };
export const Enforced: S = { args: { standing: "enforced" } };
export const Reviewed: S = { args: { standing: "reviewed" } };
export const Planned: S = { args: { standing: "planned" } };
export const Advisory: S = { args: { standing: "advisory" } };
export const Retired: S = { args: { standing: "retired" } };
export const Trial: S = { args: { standing: "enforced", mark: "trial" } };
export const Blocked: S = { args: { standing: "planned", mark: "blocked" } };
export const Unproven: S = { args: { standing: "reviewed", mark: "unproven" } };
export const Status: S = { args: { status: "active" } };
export const Observation: S = { args: { observation: "tighter" } };
