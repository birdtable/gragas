import type { Meta, Story } from "../../parts/stories.ts";
import { bundleOf, earlyDay, lateDay, snapshot } from "./fixture.ts";
import {
  AttentionRegion,
  states,
  type AttentionRegionProps,
} from "./attention.tsx";

const meta: Meta<AttentionRegionProps> = {
  title: "Views/Attention",
  states,
  component: AttentionRegion,
  parameters: { a11y: { known: [] } },
};

export default meta;
type S = Story<AttentionRegionProps>;

const retry = (): void => undefined;
const ready = {
  state: "ready",
  file: "bundle.json",
  value: bundleOf(),
} as const;

export const Default: S = {
  args: { region: ready, today: earlyDay, onRetry: retry },
};
export const Loading: S = {
  args: {
    region: { state: "loading", file: "bundle.json" },
    today: earlyDay,
    onRetry: retry,
  },
};
export const Empty: S = {
  args: {
    region: { state: "empty", file: "bundle.json", why: "no-rows" },
    today: earlyDay,
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
    today: earlyDay,
    onRetry: retry,
  },
};
export const Overdue: S = {
  args: {
    region: ready,
    today: lateDay,
    filters: { kind: "overdue" },
    onRetry: retry,
  },
};
export const Phone390: S = {
  args: { region: ready, today: earlyDay, onRetry: retry },
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
    today: earlyDay,
    onRetry: retry,
  },
};
