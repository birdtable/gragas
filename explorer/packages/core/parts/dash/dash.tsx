// The dash with its reason: an empty cell is never a bare dash. The dash is what
// a sighted reader scans; the reason is the title and the words a screen reader
// hears, so "none" always arrives with why.
import type { ReactElement } from "react";
import { say } from "../say.ts";
import "../base.css";
import "./dash.css";

export const states = ["not-applicable", "not-measured"] as const;

export type DashProps = Readonly<{ reason: string }>;

export function Dash({ reason }: DashProps): ReactElement {
  return (
    <span data-part="dash" title={reason}>
      <span aria-hidden="true">{"—"}</span>
      <span className="gr-hidden">{say("part.dash.none", { reason })}</span>
    </span>
  );
}
