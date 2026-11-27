// The dialog part: Base UI's modal dialog, which keeps the contract. Focus moves
// in when it opens and is trapped there, goes back to the trigger when it
// closes, and Escape closes only the topmost dialog. The part adds a title and
// a Close button every time, so a dialog is never nameless or a dead end.
import { Dialog as BaseDialog } from "@base-ui/react/dialog";
import type { ReactElement, ReactNode } from "react";
import { say } from "../say.ts";
import "../base.css";
import "./dialog.css";

export const states = ["closed", "open"] as const;

export type DialogProps = Readonly<{
  /** The words on the button that opens it. */
  trigger: string;
  title: string;
  description?: string;
  children?: ReactNode;
  /** Starts open (the catalogue's story for it). */
  initiallyOpen?: boolean;
}>;

export function Dialog({
  trigger,
  title,
  description,
  children,
  initiallyOpen,
}: DialogProps): ReactElement {
  return (
    <BaseDialog.Root defaultOpen={initiallyOpen ?? false}>
      <BaseDialog.Trigger
        data-part="dialog"
        data-region="trigger"
        className="gr-button gr-target"
      >
        {trigger}
      </BaseDialog.Trigger>
      <BaseDialog.Portal>
        <BaseDialog.Backdrop className="gr-dialog-backdrop" />
        <BaseDialog.Viewport className="gr-dialog-viewport">
          <BaseDialog.Popup
            data-part="dialog"
            data-region="popup"
            className="gr-dialog"
          >
            <BaseDialog.Title className="gr-dialog-title">
              {title}
            </BaseDialog.Title>
            {description === undefined ? null : (
              <BaseDialog.Description className="gr-dialog-description">
                {description}
              </BaseDialog.Description>
            )}
            <div className="gr-dialog-body">{children}</div>
            <BaseDialog.Close className="gr-button gr-target">
              {say("part.close")}
            </BaseDialog.Close>
          </BaseDialog.Popup>
        </BaseDialog.Viewport>
      </BaseDialog.Portal>
    </BaseDialog.Root>
  );
}
