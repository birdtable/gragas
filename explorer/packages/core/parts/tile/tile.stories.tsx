import { Count } from "../count/count.tsx";
import type { Meta, Story } from "../stories.ts";
import { Tile, states, type TileProps } from "./tile.tsx";

const meta: Meta<TileProps> = {
  title: "Parts/Tile",
  states,
  component: Tile,
  parameters: { a11y: { known: [] } },
};

export default meta;
type S = Story<TileProps>;

export const Plain: S = { args: { label: "Rules", value: "128" } };
export const WithNote: S = {
  args: { label: "Rules", value: "128", note: "12 proposed" },
};
export const WithPart: S = {
  args: { label: "Findings", value: <Count value={3} unit="open" /> },
};
