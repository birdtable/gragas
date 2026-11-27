// The count part: a number through the formatter (web.content.2), with the unit
// beside it when there is one. It never formats a number itself.
import type { ReactElement } from "react";
import { createFormatter } from "../../formatter/index.ts";
import "../base.css";
import "./count.css";

const format = createFormatter();

export const states = ["zero", "one", "many", "large", "with-unit"] as const;

export type CountProps = Readonly<{ value: number; unit?: string }>;

export function Count({ value, unit }: CountProps): ReactElement {
  return (
    <span data-part="count" data-value={value}>
      <span className="gr-count-value">{format.number(value)}</span>
      {unit === undefined ? null : (
        <span className="gr-count-unit"> {unit}</span>
      )}
    </span>
  );
}
