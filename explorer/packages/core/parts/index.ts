// The parts: the seventeen the explorer's views draw with. A view
// imports a part from here and nothing from a part's own folder; the parts take
// plain props and their words from the message module, with no provider.
export { Banner, type BannerProps, type Tone } from "./banner/banner.tsx";
export { Count, type CountProps } from "./count/count.tsx";
export {
  CommandPalette,
  type Command,
  type CommandPaletteProps,
} from "./command-palette/command-palette.tsx";
export {
  CopyableValue,
  type CopyableValueProps,
} from "./copyable-value/copyable-value.tsx";
export { Dash, type DashProps } from "./dash/dash.tsx";
export { Dialog, type DialogProps } from "./dialog/dialog.tsx";
export {
  EmptyFrame,
  type EmptyFrameProps,
} from "./empty-frame/empty-frame.tsx";
export {
  ErrorFrame,
  type ErrorFrameProps,
} from "./error-frame/error-frame.tsx";
export {
  FilterBar,
  type Filter,
  type FilterBarProps,
} from "./filter-bar/filter-bar.tsx";
export {
  FilterChip,
  type FilterChipProps,
} from "./filter-chip/filter-chip.tsx";
export { Link, type LinkProps } from "./link/link.tsx";
export {
  ListTable,
  type Column,
  type ListTableProps,
  type Row,
} from "./list-table/list-table.tsx";
export { SidePanel, type SidePanelProps } from "./side-panel/side-panel.tsx";
export {
  StackedStandingBar,
  type Segment,
  type StackedStandingBarProps,
} from "./stacked-standing-bar/stacked-standing-bar.tsx";
export {
  StatusMark,
  type Mark,
  type Observation,
  type Standing,
  type Status,
  type StatusMarkProps,
} from "./status-mark/status-mark.tsx";
export { Tabs, type Tab, type TabsProps } from "./tabs/tabs.tsx";
export { Tile, type TileProps } from "./tile/tile.tsx";
