// The stacked standing bar: how many rules sit in each standing, as one bar and
// as a legend. The bar is only the picture; the legend (each standing's symbol,
// word and count) is what says it, so nothing is told by colour or length alone.
import type { ReactElement } from "react";
import { standings } from "@gragas/tokens";
import { Count } from "../count/count.tsx";
import { say } from "../say.ts";
import { StatusMark, type Standing } from "../status-mark/status-mark.tsx";
import "../base.css";
import "./stacked-standing-bar.css";

export const states = ["mixed", "single", "none"] as const;

export type Segment = Readonly<{ standing: Standing; count: number }>;

export type StackedStandingBarProps = Readonly<{
  segments: readonly Segment[];
}>;

const roleOf = (standing: Standing): string =>
  standings.find((s) => s.standing === standing)?.role ?? "grey";

export function StackedStandingBar({
  segments,
}: StackedStandingBarProps): ReactElement {
  const shown = segments.filter((s) => s.count > 0);
  const total = shown.reduce((sum, s) => sum + s.count, 0);
  return (
    <figure data-part="stacked-standing-bar" aria-label={say("part.bar.label")}>
      <svg
        className="gr-bar"
        viewBox="0 0 100 6"
        preserveAspectRatio="none"
        aria-hidden="true"
        focusable="false"
      >
        <rect className="gr-bar-track" x="0" y="0" width="100" height="6" />
        {shown.map((segment, index) => {
          const width = (segment.count / total) * 100;
          const x =
            (shown
              .slice(0, index)
              .reduce((sum, earlier) => sum + earlier.count, 0) /
              total) *
            100;
          return (
            <rect
              key={segment.standing}
              className="gr-bar-segment"
              data-standing={segment.standing}
              data-role={roleOf(segment.standing)}
              x={x}
              y="0"
              width={width}
              height="6"
            />
          );
        })}
      </svg>
      <ul className="gr-bar-legend">
        {segments.map((segment) => (
          <li key={segment.standing}>
            <StatusMark standing={segment.standing} />{" "}
            <Count value={segment.count} />
          </li>
        ))}
      </ul>
    </figure>
  );
}
