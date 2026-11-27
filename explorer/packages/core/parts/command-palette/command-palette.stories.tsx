import type { Meta, Story } from "../stories.ts";
import {
  CommandPalette,
  states,
  type CommandPaletteProps,
} from "./command-palette.tsx";

const meta: Meta<CommandPaletteProps> = {
  title: "Parts/Command palette shell",
  states,
  component: CommandPalette,
  parameters: { a11y: { known: [] } },
};

export default meta;
type S = Story<CommandPaletteProps>;

const base = {
  trigger: "Commands",
  onRun: (): void => undefined,
  commands: [
    { id: "rules", label: "Go to rules" },
    { id: "tools", label: "Go to tools" },
    { id: "theme", label: "Switch theme" },
  ],
};

export const Closed: S = { args: base };
export const All: S = { args: { ...base, initiallyOpen: true } };
export const Filtered: S = {
  args: { ...base, initiallyOpen: true, initialQuery: "go to" },
};
export const NoMatch: S = {
  args: { ...base, initiallyOpen: true, initialQuery: "zzz" },
};
