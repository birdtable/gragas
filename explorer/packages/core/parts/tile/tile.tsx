// The tile part: one labelled figure, a term and its value, with a note under it
// when there is one. A definition list, so a screen reader hears label then value.
import type { ReactElement, ReactNode } from "react";
import "../base.css";
import "./tile.css";

export const states = ["plain", "with-note", "with-part"] as const;

export type TileProps = Readonly<{
  label: string;
  value: ReactNode;
  note?: string;
}>;

export function Tile({ label, value, note }: TileProps): ReactElement {
  return (
    <dl data-part="tile">
      <dt className="gr-tile-label">{label}</dt>
      <dd className="gr-tile-value">{value}</dd>
      {note === undefined ? null : <dd className="gr-tile-note">{note}</dd>}
    </dl>
  );
}
