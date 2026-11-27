// The filter chip: one applied filter, its name and value, and a button that
// removes it. The button's name says which filter it removes.
import { Button } from "@base-ui/react/button";
import type { ReactElement } from "react";
import { Icon } from "../icons.tsx";
import { say } from "../say.ts";
import "../base.css";
import "./filter-chip.css";

export const states = ["applied"] as const;

export type FilterChipProps = Readonly<{
  name: string;
  value: string;
  onRemove: () => void;
}>;

export function FilterChip({
  name,
  value,
  onRemove,
}: FilterChipProps): ReactElement {
  return (
    <span data-part="filter-chip" data-filter={name}>
      <span className="gr-chip-name">{name}</span>
      <span className="gr-chip-value">{value}</span>
      <Button
        className="gr-chip-remove gr-target"
        aria-label={say("part.filter.remove", { name })}
        onClick={onRemove}
      >
        <Icon name="close" />
      </Button>
    </span>
  );
}
