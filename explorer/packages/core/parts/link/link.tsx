// The link part: an anchor. One that leaves the page opens a new tab and says so
// in words a screen reader reads, with rel set so the new page gets no handle on
// this one.
import type { ReactElement, ReactNode } from "react";
import { say } from "../say.ts";
import "../base.css";
import "./link.css";

export const states = ["internal", "external"] as const;

export type LinkProps = Readonly<{
  href: string;
  external?: boolean;
  children: ReactNode;
}>;

export function Link({ href, external, children }: LinkProps): ReactElement {
  if (external === true) {
    return (
      <a
        data-part="link"
        className="gr-target"
        href={href}
        target="_blank"
        rel="noopener noreferrer"
      >
        {children}
        <span className="gr-hidden"> ({say("part.link.new-tab")})</span>
      </a>
    );
  }
  return (
    <a data-part="link" className="gr-target" href={href}>
      {children}
    </a>
  );
}
