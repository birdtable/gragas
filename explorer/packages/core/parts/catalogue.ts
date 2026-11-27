// The catalogue's registry: every part and the module of its
// stories. Storybook reads the stories from their folders; the catalogue test
// reads this list, so a part with no entry here, or an entry whose stories miss
// a state, fails it. Adding a part is one line here.
import * as banner from "./banner/banner.stories.tsx";
import * as commandPalette from "./command-palette/command-palette.stories.tsx";
import * as copyableValue from "./copyable-value/copyable-value.stories.tsx";
import * as count from "./count/count.stories.tsx";
import * as dash from "./dash/dash.stories.tsx";
import * as dialog from "./dialog/dialog.stories.tsx";
import * as emptyFrame from "./empty-frame/empty-frame.stories.tsx";
import * as errorFrame from "./error-frame/error-frame.stories.tsx";
import * as filterBar from "./filter-bar/filter-bar.stories.tsx";
import * as filterChip from "./filter-chip/filter-chip.stories.tsx";
import * as link from "./link/link.stories.tsx";
import * as listTable from "./list-table/list-table.stories.tsx";
import * as sidePanel from "./side-panel/side-panel.stories.tsx";
import * as stackedStandingBar from "./stacked-standing-bar/stacked-standing-bar.stories.tsx";
import * as statusMark from "./status-mark/status-mark.stories.tsx";
import * as tabs from "./tabs/tabs.stories.tsx";
import * as tile from "./tile/tile.stories.tsx";

export type StoryModule = Readonly<Record<string, unknown>>;

export const catalogue: Readonly<Record<string, StoryModule>> = {
  banner,
  "command-palette": commandPalette,
  "copyable-value": copyableValue,
  count,
  dash,
  dialog,
  "empty-frame": emptyFrame,
  "error-frame": errorFrame,
  "filter-bar": filterBar,
  "filter-chip": filterChip,
  link,
  "list-table": listTable,
  "side-panel": sidePanel,
  "stacked-standing-bar": stackedStandingBar,
  "status-mark": statusMark,
  tabs,
  tile,
};
