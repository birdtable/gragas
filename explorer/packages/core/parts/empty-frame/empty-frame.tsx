// The empty frame: what a region says when it has nothing to show. A title
// always, and a hint when the reader can do something about it.
import type { ReactElement, ReactNode } from "react";
import "../base.css";
import "./empty-frame.css";

export const states = ["title-only", "with-hint"] as const;

export type EmptyFrameProps = Readonly<{
  title: string;
  children?: ReactNode;
}>;

export function EmptyFrame({ title, children }: EmptyFrameProps): ReactElement {
  return (
    <div data-part="empty-frame">
      <p className="gr-frame-title">{title}</p>
      {children === undefined ? null : (
        <div className="gr-frame-hint">{children}</div>
      )}
    </div>
  );
}
