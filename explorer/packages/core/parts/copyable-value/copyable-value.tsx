// The copyable value: a value in mono with a button that copies it. The button's
// word changes to Copied and a polite live region says so, so the change is not
// only seen.
import { Button } from "@base-ui/react/button";
import { useState, type ReactElement } from "react";
import { Icon } from "../icons.tsx";
import { say } from "../say.ts";
import "../base.css";
import "./copyable-value.css";

export const states = ["idle", "copied"] as const;

export type CopyableValueProps = Readonly<{
  value: string;
  /** Starts in the copied state (the catalogue's story for it). */
  copied?: boolean;
}>;

export function CopyableValue({
  value,
  copied,
}: CopyableValueProps): ReactElement {
  const [done, setDone] = useState(copied ?? false);
  return (
    <span data-part="copyable-value">
      <code className="gr-copy-value">{value}</code>
      <Button
        className="gr-button gr-target"
        aria-label={`${done ? say("part.copied") : say("part.copy")} ${value}`}
        onClick={() => {
          navigator.clipboard.writeText(value).then(
            () => {
              setDone(true);
            },
            () => {
              setDone(false);
            },
          );
        }}
      >
        <Icon name={done ? "check" : "copy"} />
        {done ? say("part.copied") : say("part.copy")}
      </Button>
      <span className="gr-hidden" role="status">
        {done ? say("part.copied") : ""}
      </span>
    </span>
  );
}
