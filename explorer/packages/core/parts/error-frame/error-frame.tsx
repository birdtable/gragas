// The error frame: what a region says when its load failed. An alert that
// states what happened and offers Retry, a real button a keyboard reaches.
import { Button } from "@base-ui/react/button";
import type { ReactElement } from "react";
import { say } from "../say.ts";
import "../base.css";
import "./error-frame.css";

export const states = ["with-retry"] as const;

export type ErrorFrameProps = Readonly<{
  message: string;
  onRetry: () => void;
}>;

export function ErrorFrame({
  message,
  onRetry,
}: ErrorFrameProps): ReactElement {
  return (
    <div data-part="error-frame" role="alert">
      <p className="gr-frame-title">{say("part.error.heading")}</p>
      <p className="gr-frame-hint">{message}</p>
      <Button className="gr-button gr-target" onClick={onRetry}>
        {say("part.retry")}
      </Button>
    </div>
  );
}
