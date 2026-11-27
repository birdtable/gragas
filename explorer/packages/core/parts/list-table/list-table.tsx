// The list table: a captioned table of rows. Every column header is a real
// header cell, numbers align right, and no rows is the empty frame, never a
// header with nothing under it.
import type { ReactElement, ReactNode } from "react";
import { EmptyFrame } from "../empty-frame/empty-frame.tsx";
import { say } from "../say.ts";
import "../base.css";
import "./list-table.css";

export const states = ["rows", "one-row", "empty"] as const;

export type Column = Readonly<{
  key: string;
  label: string;
  numeric?: boolean;
}>;

export type Row = Readonly<{
  id: string;
  cells: Readonly<Record<string, ReactNode>>;
}>;

export type ListTableProps = Readonly<{
  caption: string;
  columns: readonly Column[];
  rows: readonly Row[];
}>;

export function ListTable({
  caption,
  columns,
  rows,
}: ListTableProps): ReactElement {
  if (rows.length === 0) {
    return (
      <div data-part="list-table" data-empty="">
        <EmptyFrame title={say("part.table.empty")} />
      </div>
    );
  }
  return (
    <div data-part="list-table">
      <table>
        <caption>{caption}</caption>
        <thead>
          <tr>
            {columns.map((column) => (
              <th
                key={column.key}
                scope="col"
                data-numeric={column.numeric === true ? "" : undefined}
              >
                {column.label}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => (
            <tr key={row.id} data-row={row.id}>
              {columns.map((column) => (
                <td
                  key={column.key}
                  data-numeric={column.numeric === true ? "" : undefined}
                >
                  {row.cells[column.key]}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
