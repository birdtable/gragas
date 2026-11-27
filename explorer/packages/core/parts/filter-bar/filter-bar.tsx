// The filter bar: the applied filters as chips in a labelled group, and a Clear
// button once there is more than nothing to clear.
import { Button } from "@base-ui/react/button";
import type { ReactElement } from "react";
import { FilterChip } from "../filter-chip/filter-chip.tsx";
import { say } from "../say.ts";
import "../base.css";
import "./filter-bar.css";

export const states = ["none", "one", "several"] as const;

export type Filter = Readonly<{ name: string; value: string }>;

export type FilterBarProps = Readonly<{
  filters: readonly Filter[];
  onRemove: (name: string) => void;
  onClear: () => void;
}>;

export function FilterBar({
  filters,
  onRemove,
  onClear,
}: FilterBarProps): ReactElement {
  return (
    <div data-part="filter-bar" role="group" aria-label={say("part.filters")}>
      {filters.map((filter) => (
        <FilterChip
          key={filter.name}
          name={filter.name}
          value={filter.value}
          onRemove={() => {
            onRemove(filter.name);
          }}
        />
      ))}
      {filters.length === 0 ? null : (
        <Button className="gr-button gr-target" onClick={onClear}>
          {say("part.filters.clear")}
        </Button>
      )}
    </div>
  );
}
