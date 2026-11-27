import { Dash } from "../dash/dash.tsx";
import { StatusMark } from "../status-mark/status-mark.tsx";
import type { Meta, Story } from "../stories.ts";
import { ListTable, states, type ListTableProps } from "./list-table.tsx";

const meta: Meta<ListTableProps> = {
  title: "Parts/List table",
  states,
  component: ListTable,
  parameters: { a11y: { known: [] } },
};

export default meta;
type S = Story<ListTableProps>;

const columns = [
  { key: "rule", label: "Rule" },
  { key: "standing", label: "Standing" },
  { key: "findings", label: "Findings", numeric: true },
] as const;

export const Rows: S = {
  args: {
    caption: "Rules",
    columns,
    rows: [
      {
        id: "go.errors.1",
        cells: {
          rule: "go.errors.1",
          standing: <StatusMark standing="enforced" />,
          findings: "0",
        },
      },
      {
        id: "go.errors.2",
        cells: {
          rule: "go.errors.2",
          standing: <StatusMark standing="planned" mark="blocked" />,
          findings: <Dash reason="not measured yet" />,
        },
      },
    ],
  },
};
export const OneRow: S = {
  args: {
    caption: "Rules",
    columns,
    rows: [
      {
        id: "go.errors.1",
        cells: {
          rule: "go.errors.1",
          standing: <StatusMark standing="proposed" />,
          findings: "4",
        },
      },
    ],
  },
};
export const Empty: S = { args: { caption: "Rules", columns, rows: [] } };
