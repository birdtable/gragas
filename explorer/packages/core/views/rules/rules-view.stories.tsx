// The rules view's stories: one per state of a data region, and the
// phone width, each a set of props the view is drawn with. Sample data is the
// fixture bundle the Go side ships.
import type { Meta, Story } from "../../parts/stories.ts";
import { RulesView, states, type RulesViewProps } from "./rules-view.tsx";
import { fixtureBundle } from "./rules-view.fixture.ts";

const meta: Meta<RulesViewProps> = {
  title: "Views/Rules",
  states,
  component: RulesView,
  parameters: { a11y: { known: [] } },
};

export default meta;
type S = Story<RulesViewProps>;

const never = (): void => undefined;
const ready = {
  state: "ready",
  file: "bundle.json",
  value: fixtureBundle,
} as const;

const base = { filters: {}, onFilters: never, onRetry: never } as const;

/** The six states of a data region, each by its own name (ready is the default). */
export const Default: S = { args: { ...base, region: ready } };
export const Loading: S = {
  args: { ...base, region: { state: "loading", file: "bundle.json" } },
};
export const Empty: S = {
  args: {
    ...base,
    region: {
      state: "empty",
      file: "bundle.json",
      why: "no-rows",
    },
  },
};
export const Error: S = {
  args: {
    ...base,
    region: {
      state: "error",
      file: "bundle.json",
      cause: { kind: "unexpected", file: "bundle.json", message: "bad json" },
      retry: true,
    },
  },
};
export const Stale: S = {
  args: {
    ...base,
    region: {
      state: "stale",
      file: "bundle.json",
      value: fixtureBundle,
      ageMs: 300_000,
      cause: { kind: "disconnected", file: "bundle.json" },
      retry: true,
    },
  },
};
export const Disconnected: S = {
  args: {
    ...base,
    region: { state: "disconnected", file: "bundle.json", retry: true },
  },
};

/** Filters in force: the chips, the clear-all and the narrowed counts. */
export const Filtered: S = {
  args: {
    ...base,
    region: ready,
    filters: { part: "go", strength: "must", q: "error" },
  },
};

/** The phone width (390 px): the filters stack and a table scrolls inside its group. */
export const Phone390: S = { args: { ...base, region: ready } };
