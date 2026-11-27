// The standing view's page: one stacked bar per group with each
// standing's word, symbol and count, the marks beside a standing as their own
// segments with their word, and the filters and grouping. `StandingPage` is the
// one orchestrator; `StandingBody` takes plain props and draws the six states of
// a data region (web.structure.4), alone when `embedded`: then it is the one
// stacked bar of the chain and nothing of the breakdown.
import { Button } from "@base-ui/react/button";
import { skipToken, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ReactElement } from "react";
import {
  freshness,
  keys,
  retryLoad,
  type BundleValue,
  type DataState,
  type Loaded,
} from "../../data/index.ts";
import { createFormatter } from "../../formatter/index.ts";
import {
  Banner,
  Count,
  Dash,
  EmptyFrame,
  ErrorFrame,
  FilterBar,
  StackedStandingBar,
  StatusMark,
  Tile,
} from "../../parts/index.ts";
import { say } from "../../parts/say.ts";
import { bundleRegion } from "../../selectors/index.ts";
import type { StandingSearch } from "./search.ts";
import {
  standingView,
  type Bar,
  type Group,
  type StandingQuery,
  type StandingView,
} from "./selectors.ts";
import "../../parts/base.css";
import "./standing.css";

export const states = [
  "ready",
  "loading",
  "empty",
  "error",
  "stale",
  "disconnected",
] as const;

const format = createFormatter();

export type StandingBodyProps = Readonly<{
  state: DataState<StandingView>;
  /** What the address carries now: the next address is built from it. */
  query: StandingQuery;
  onQuery: (next: StandingQuery) => void;
  onRetry: () => void;
  /** The one bar of the chain, with no breakdown and no controls. */
  embedded?: boolean;
}>;

/** The query with `patch` laid over it and every cleared key dropped. */
function patched(query: StandingQuery, patch: StandingQuery): StandingQuery {
  const next: Record<string, unknown> = { ...query, ...patch };
  for (const key of Object.keys(next)) {
    if (next[key] === undefined) Reflect.deleteProperty(next, key);
  }
  return next;
}

function Marked({ bar }: { readonly bar: Bar }): ReactElement {
  return bar.marked.length === 0 ? (
    <p className="gr-standing-note">{say("standing.marks.none")}</p>
  ) : (
    <ul className="gr-standing-marks" aria-label={say("standing.marks")}>
      {bar.marked.map((m) => (
        <li
          key={`${m.standing}/${m.mark}`}
          data-standing={m.standing}
          data-mark={m.mark}
        >
          <StatusMark standing={m.standing} mark={m.mark} />{" "}
          <Count value={m.count} />
        </li>
      ))}
    </ul>
  );
}

function GroupView({ group }: { readonly group: Group }): ReactElement {
  return (
    <section data-region="group" data-group={group.id}>
      <h2 className="gr-standing-group">
        {group.key.kind === "value" ? (
          group.key.value
        ) : (
          <Dash reason={say(group.key.reason)} />
        )}
      </h2>
      <StackedStandingBar segments={group.segments} />
      <Marked bar={group} />
    </section>
  );
}

function Toggle({
  pressed,
  label,
  onPress,
}: {
  readonly pressed: boolean;
  readonly label: string;
  readonly onPress: () => void;
}): ReactElement {
  return (
    <Button
      className="gr-button gr-target"
      aria-pressed={pressed}
      onClick={onPress}
    >
      {label}
    </Button>
  );
}

function Controls({
  view,
  query,
  onQuery,
}: {
  readonly view: StandingView;
  readonly query: StandingQuery;
  readonly onQuery: (next: StandingQuery) => void;
}): ReactElement {
  return (
    <div data-region="controls">
      <div role="group" aria-label={say("standing.group.label")}>
        <span className="gr-standing-label">{say("standing.group.label")}</span>
        {(["part", "kind"] as const).map((by) => (
          <Toggle
            key={by}
            pressed={view.by === by}
            label={say(`standing.group.${by}`)}
            onPress={() => {
              onQuery(patched(query, { by: by === "part" ? undefined : by }));
            }}
          />
        ))}
      </div>
      <div role="group" aria-label={say("standing.filter.part")}>
        <span className="gr-standing-label">{say("standing.filter.part")}</span>
        {view.parts.map((p) => (
          <Toggle
            key={p}
            pressed={query.part === p}
            label={p}
            onPress={() => {
              onQuery(
                patched(query, { part: query.part === p ? undefined : p }),
              );
            }}
          />
        ))}
      </div>
      <div role="group" aria-label={say("standing.filter.kind")}>
        <span className="gr-standing-label">{say("standing.filter.kind")}</span>
        {view.kinds.map((k) => (
          <Toggle
            key={k}
            pressed={query.kind === k}
            label={k}
            onPress={() => {
              onQuery(
                patched(query, { kind: query.kind === k ? undefined : k }),
              );
            }}
          />
        ))}
      </div>
      <FilterBar
        filters={view.applied}
        onRemove={(name) => {
          onQuery(patched(query, { [name]: undefined }));
        }}
        onClear={() => {
          onQuery(patched(query, { part: undefined, kind: undefined }));
        }}
      />
    </div>
  );
}

function Ready({
  view,
  query,
  onQuery,
  embedded,
}: {
  readonly view: StandingView;
  readonly query: StandingQuery;
  readonly onQuery: (next: StandingQuery) => void;
  readonly embedded: boolean;
}): ReactElement {
  const chain = (
    <section data-region="chain">
      {embedded ? null : (
        <h2 className="gr-standing-group">{say("standing.chain")}</h2>
      )}
      <Tile
        label={say("count.rules", { count: view.chain.total })}
        value={<Count value={view.chain.total} />}
      />
      <StackedStandingBar segments={view.chain.segments} />
      <Marked bar={view.chain} />
    </section>
  );
  if (embedded) return chain;
  return (
    <>
      <Controls view={view} query={query} onQuery={onQuery} />
      {chain}
      <div
        data-region="groups"
        aria-label={say("standing.bars", {
          by: say(`standing.group.${view.by}`),
        })}
      >
        {view.groups.length === 0 ? (
          <EmptyFrame title={say("standing.nothing-left")} />
        ) : (
          view.groups.map((group) => <GroupView key={group.id} group={group} />)
        )}
      </div>
    </>
  );
}

/** Draws one of the six states of the standing region: every failure offers Retry. */
export function StandingBody({
  state,
  query,
  onQuery,
  onRetry,
  embedded = false,
}: StandingBodyProps): ReactElement {
  return (
    <section data-view="standing" data-state={state.state}>
      <h1>{say("standing.heading")}</h1>
      {state.state === "loading" ? (
        <p role="status">{say("standing.loading")}</p>
      ) : state.state === "empty" ? (
        <EmptyFrame title={say("standing.empty")}>
          {say("standing.empty.hint")}
        </EmptyFrame>
      ) : state.state === "error" || state.state === "disconnected" ? (
        <ErrorFrame message={say("standing.error")} onRetry={onRetry} />
      ) : (
        <>
          {state.state === "stale" ? (
            <Banner tone="warning">
              {say("standing.stale", { age: format.duration(state.ageMs) })}
            </Banner>
          ) : null}
          {embedded ? null : (
            <p className="gr-standing-note">{say("standing.intro")}</p>
          )}
          <Ready
            view={state.value}
            query={query}
            onQuery={onQuery}
            embedded={embedded}
          />
        </>
      )}
    </section>
  );
}

export type StandingPageProps = Readonly<{
  search: StandingSearch;
  params?: Readonly<Record<string, string>>;
  onSearch?: (next: StandingSearch) => void;
}>;

/** The one orchestrator: the address in, the loaded bundle in, plain values out. */
export function StandingPage({
  search,
  onSearch,
}: StandingPageProps): ReactElement {
  const client = useQueryClient();
  // The shell has loaded the bundle before it draws a page; this only reads it.
  const snapshot = useQuery<Loaded<BundleValue>>({
    queryKey: keys.bundle(),
    queryFn: skipToken,
    ...freshness,
  });
  const region = bundleRegion(snapshot, snapshot.dataUpdatedAt);
  const state: DataState<StandingView> =
    region.state === "ready" || region.state === "stale"
      ? { ...region, value: standingView(region.value, search) }
      : region;
  return (
    <StandingBody
      state={state}
      query={search}
      onQuery={(next) => {
        onSearch?.(next);
      }}
      onRetry={() => {
        void retryLoad(client, "bundle.json");
      }}
    />
  );
}
