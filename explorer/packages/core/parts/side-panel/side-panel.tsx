// The side panel: a labelled complementary region with a heading and a Close
// button. It sits beside the page, so it is no dialog and takes no focus on its own.
import { Button } from "@base-ui/react/button";
import type { ReactElement, ReactNode } from "react";
import { Icon } from "../icons.tsx";
import { say } from "../say.ts";
import "../base.css";
import "./side-panel.css";

export const states = ["short", "long"] as const;

export type SidePanelProps = Readonly<{
  title: string;
  onClose: () => void;
  children: ReactNode;
}>;

export function SidePanel({
  title,
  onClose,
  children,
}: SidePanelProps): ReactElement {
  return (
    <aside data-part="side-panel" aria-label={title}>
      <header className="gr-panel-head">
        <h2 className="gr-panel-title">{title}</h2>
        <Button
          className="gr-button gr-target"
          aria-label={say("part.panel.close")}
          onClick={onClose}
        >
          <Icon name="close" />
        </Button>
      </header>
      <div className="gr-panel-body">{children}</div>
    </aside>
  );
}
