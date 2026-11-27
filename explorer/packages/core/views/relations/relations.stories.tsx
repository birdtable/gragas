import type { Meta, Story } from "../../parts/stories.ts";
import { bundleOf, snapshot } from "./fixture.ts";
import {
  RelationsRegion,
  states,
  type RelationsRegionProps,
} from "./relations.tsx";

const meta: Meta<RelationsRegionProps> = {
  title: "Views/Relations",
  states,
  component: RelationsRegion,
  parameters: { a11y: { known: [] } },
};

export default meta;
type S = Story<RelationsRegionProps>;

const retry = (): void => undefined;
const around = { kind: "rule", id: "go.errors.1" } as const;
const ready = {
  state: "ready",
  file: "bundle.json",
  value: bundleOf(),
} as const;

export const Default: S = {
  args: { region: ready, ...around, onExpand: retry, onRetry: retry },
};
export const Loading: S = {
  args: {
    region: { state: "loading", file: "bundle.json" },
    ...around,
    onRetry: retry,
  },
};
export const Empty: S = {
  args: {
    region: { state: "empty", file: "bundle.json", why: "no-rows" },
    ...around,
    onRetry: retry,
  },
};
export const Error: S = {
  args: {
    region: {
      state: "error",
      file: "bundle.json",
      cause: snapshot(),
      retry: true,
    },
    ...around,
    onRetry: retry,
  },
};
export const Phone390: S = {
  args: { region: ready, ...around, onExpand: retry, onRetry: retry },
};
export const Stale: S = {
  args: {
    region: {
      state: "stale",
      file: "bundle.json",
      value: bundleOf(),
      ageMs: 120_000,
      cause: snapshot(),
      retry: true,
    },
    ...around,
    onRetry: retry,
  },
};
