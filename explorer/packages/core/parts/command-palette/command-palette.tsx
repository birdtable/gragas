// The command palette shell: a dialog (so focus goes in, is trapped and comes
// back) holding a search input and the commands that match it as buttons. Enter
// in the input runs the first match; each command is also a button a keyboard
// reaches by Tab. The shell runs nothing itself: the page passes `onRun`.
import { Button } from "@base-ui/react/button";
import { Dialog as BaseDialog } from "@base-ui/react/dialog";
import { Input } from "@base-ui/react/input";
import { useState, type ReactElement } from "react";
import { say } from "../say.ts";
import "../base.css";
import "./command-palette.css";

export const states = ["closed", "all", "filtered", "no-match"] as const;

export type Command = Readonly<{ id: string; label: string }>;

export type CommandPaletteProps = Readonly<{
  /** The words on the button that opens it. */
  trigger: string;
  commands: readonly Command[];
  onRun: (id: string) => void;
  /** Starts open (the catalogue's story for it). */
  initiallyOpen?: boolean;
  /** Starts with this text in the search input. */
  initialQuery?: string;
}>;

export function CommandPalette({
  trigger,
  commands,
  onRun,
  initiallyOpen,
  initialQuery,
}: CommandPaletteProps): ReactElement {
  const [query, setQuery] = useState(initialQuery ?? "");
  const [open, setOpen] = useState(initiallyOpen ?? false);
  const needle = query.trim().toLowerCase();
  const matches = commands.filter((c) =>
    c.label.toLowerCase().includes(needle),
  );
  const run = (id: string): void => {
    onRun(id);
    setOpen(false);
  };
  return (
    <BaseDialog.Root open={open} onOpenChange={setOpen}>
      <BaseDialog.Trigger
        data-part="command-palette"
        data-region="trigger"
        className="gr-button gr-target"
      >
        {trigger}
      </BaseDialog.Trigger>
      <BaseDialog.Portal>
        <BaseDialog.Backdrop className="gr-dialog-backdrop" />
        <BaseDialog.Viewport className="gr-dialog-viewport">
          <BaseDialog.Popup
            data-part="command-palette"
            data-region="popup"
            className="gr-dialog gr-palette"
          >
            <BaseDialog.Title className="gr-dialog-title">
              {say("part.palette.label")}
            </BaseDialog.Title>
            <Input
              type="search"
              className="gr-palette-input gr-target"
              aria-label={say("part.palette.label")}
              placeholder={say("part.palette.placeholder")}
              value={query}
              onValueChange={setQuery}
              onKeyDown={(event) => {
                const first = matches[0];
                if (event.key === "Enter" && first !== undefined) {
                  event.preventDefault();
                  run(first.id);
                }
              }}
            />
            {matches.length === 0 ? (
              <p className="gr-palette-empty" role="status">
                {say("part.palette.empty")}
              </p>
            ) : (
              <ul className="gr-palette-list">
                {matches.map((command) => (
                  <li key={command.id}>
                    <Button
                      className="gr-palette-command gr-target"
                      onClick={() => {
                        run(command.id);
                      }}
                    >
                      {command.label}
                    </Button>
                  </li>
                ))}
              </ul>
            )}
            <BaseDialog.Close className="gr-button gr-target">
              {say("part.close")}
            </BaseDialog.Close>
          </BaseDialog.Popup>
        </BaseDialog.Viewport>
      </BaseDialog.Portal>
    </BaseDialog.Root>
  );
}
