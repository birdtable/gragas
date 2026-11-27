// The tools view's page: the heading, the filters and the grouping,
// the counts, a table of the tools with a standing bar each, the entries of each
// group with the rules that hold them, the entries nothing holds, and a side
// panel for the open tool. `ToolsPage` is the one orchestrator: it reads the
// loaded bundle and the address and hands plain values to `ToolsBody`, which
// draws the six states of a data region and nothing else (web.structure.4).
import { Button } from "@base-ui/react/button";
import { skipToken, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ReactElement } from "react";
import {
  destinationPath,
  freshness,
  keys,
  retryLoad,
  type BundleValue,
  type Loaded,
  type DataState,
} from "../../data/index.ts";
import { createFormatter } from "../../formatter/index.ts";
import { bundleRegion } from "../../selectors/index.ts";
import {
  Banner,
  Count,
  Dash,
  EmptyFrame,
  ErrorFrame,
  FilterBar,
  Link,
  ListTable,
  SidePanel,
  StackedStandingBar,
  StatusMark,
  Tile,
  type Row,
} from "../../parts/index.ts";
import { say } from "../../parts/say.ts";
import type { Dashable } from "../../messages/index.ts";
import type { ToolsSearch } from "./search.ts";
import {
  toolsView,
  type EntryRow,
  type Fold,
  type Group,
  type ToolRow,
  type ToolsQuery,
  type ToolsView,
} from "./selectors.ts";
import "../../parts/base.css";
import "./tools.css";

export const states = [
  "ready",
  "loading",
  "empty",
  "error",
  "stale",
  "disconnected",
] as const;

const format = createFormatter();

export type ToolsBodyProps = Readonly<{
  state: DataState<ToolsView>;
  /** What the address carries now: the next address is built from it. */
  query: ToolsQuery;
  onQuery: (next: ToolsQuery) => void;
  onRetry: () => void;
}>;

/** The query with `patch` laid over it and every cleared key dropped. */
function patched(query: ToolsQuery, patch: ToolsQuery): ToolsQuery {
  const next: Record<string, unknown> = { ...query, ...patch };
  for (const key of Object.keys(next)) {
    if (next[key] === undefined) Reflect.deleteProperty(next, key);
  }
  return next;
}

const href = (rule: string): string =>
  `#${destinationPath({ route: "rule", id: rule })}`;

function Shown({ value }: { readonly value: Dashable<string> }): ReactElement {
  return value.kind === "value" ? (
    <span>{value.value}</span>
  ) : (
    <Dash reason={say(value.reason)} />
  );
}

function Holders({ entry }: { readonly entry: EntryRow }): ReactElement {
  return (
    <ul className="gr-tools-holders">
      {entry.holders.map((h) => (
        <li key={`${h.rule}/${String(h.rung)}`}>
          <Link href={href(h.rule)}>{h.rule}</Link>{" "}
          <span className="gr-tools-rung">
            {say("tools.rung", { rung: h.rung })}
          </span>
        </li>
      ))}
    </ul>
  );
}

function entryRows(entries: readonly EntryRow[]): Row[] {
  return entries.map((entry) => ({
    id: entry.id,
    cells: {
      entry: entry.key,
      kind: entry.kind,
      value: (
        <Shown
          value={
            entry.holders[0]?.value ?? {
              kind: "dash",
              reason: "absent.none-recorded",
            }
          }
        />
      ),
      rules: <Holders entry={entry} />,
    },
  }));
}

const entryColumns = [
  { key: "entry", label: say("tools.col.entry") },
  { key: "kind", label: say("tools.col.kind") },
  { key: "value", label: say("tools.col.value") },
  { key: "rules", label: say("tools.col.rules-holding") },
] as const;

function FoldView({
  fold,
  group,
}: {
  readonly fold: Fold;
  readonly group: string;
}): ReactElement {
  return (
    <details data-region="fold" data-rule={fold.rule}>
      <summary className="gr-target">
        {say("tools.fold.summary", {
          rule: fold.rule,
          count: format.number(fold.entries.length),
        })}
      </summary>
      <ListTable
        caption={say("tools.entries.caption", {
          group: `${group}, ${fold.rule}`,
        })}
        columns={entryColumns}
        rows={entryRows(fold.entries)}
      />
    </details>
  );
}

function GroupView({
  group,
  grouping,
}: {
  readonly group: Group;
  readonly grouping: string;
}): ReactElement {
  return (
    <section data-region="group" data-group={group.key} aria-label={group.key}>
      <h2 className="gr-tools-group">{group.key}</h2>
      {group.entries.length === 0 ? null : (
        <ListTable
          caption={say("tools.entries.caption", { group: group.key })}
          columns={entryColumns}
          rows={entryRows(group.entries)}
        />
      )}
      {group.folds.map((fold) => (
        <FoldView key={fold.rule} fold={fold} group={group.key} />
      ))}
      {grouping === "tool" && group.unmapped > 0 ? (
        <p data-region="unmapped">
          <StatusMark observation="unmapped" />{" "}
          {say("tools.unmapped.row", { count: format.number(group.unmapped) })}
          {" · "}
          {say("tools.unmapped.names")}:{" "}
          <Dash reason={say("absent.not-in-bundle")} />
        </p>
      ) : null}
    </section>
  );
}

function Panel({
  tool,
  onClose,
}: {
  readonly tool: ToolRow;
  readonly onClose: () => void;
}): ReactElement {
  return (
    <SidePanel
      title={say("tools.panel.title", { tool: tool.tool })}
      onClose={onClose}
    >
      <dl className="gr-tools-facts">
        <dt>{say("tools.col.version")}</dt>
        <dd>
          <Shown value={tool.version} />
        </dd>
        <dt>{say("tools.col.adapter")}</dt>
        <dd>
          <Shown value={tool.adapter} />
        </dd>
        <dt>{say("tools.col.entries")}</dt>
        <dd>
          <Count value={tool.entries} />
        </dd>
        <dt>{say("tools.col.rules")}</dt>
        <dd>
          <Count value={tool.rules} />
        </dd>
      </dl>
      <Link
        href={`#${destinationPath({ route: "relations", kind: "tool-entry", id: tool.tool })}`}
      >
        {say("shell.view.relations")}
      </Link>
    </SidePanel>
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

function Ready({
  view,
  query,
  onQuery,
}: {
  readonly view: ToolsView;
  readonly query: ToolsQuery;
  readonly onQuery: (next: ToolsQuery) => void;
}): ReactElement {
  const rows: Row[] = view.tools.map((tool) => ({
    id: tool.tool,
    cells: {
      tool: (
        <Button
          className="gr-button gr-target"
          aria-label={say("tools.open", { tool: tool.tool })}
          onClick={() => {
            onQuery(patched(query, { open: tool.tool }));
          }}
        >
          {tool.tool}
        </Button>
      ),
      version: <Shown value={tool.version} />,
      adapter: <Shown value={tool.adapter} />,
      entries: <Count value={tool.entries} />,
      rules: <Count value={tool.rules} />,
      standing: <StackedStandingBar segments={tool.standings} />,
    },
  }));
  return (
    <>
      <div data-region="controls">
        <div role="group" aria-label={say("tools.group.label")}>
          <span className="gr-tools-label">{say("tools.group.label")}</span>
          {(["tool", "kind"] as const).map((g) => (
            <Toggle
              key={g}
              pressed={view.grouping === g}
              label={say(`tools.group.${g}`)}
              onPress={() => {
                onQuery(
                  patched(query, { group: g === "tool" ? undefined : g }),
                );
              }}
            />
          ))}
        </div>
        <div role="group" aria-label={say("tools.filter.tool")}>
          <span className="gr-tools-label">{say("tools.filter.tool")}</span>
          {view.tools.map((t) => (
            <Toggle
              key={t.tool}
              pressed={query.tool === t.tool}
              label={t.tool}
              onPress={() => {
                onQuery(
                  patched(query, {
                    tool: query.tool === t.tool ? undefined : t.tool,
                  }),
                );
              }}
            />
          ))}
        </div>
        <div role="group" aria-label={say("tools.filter.kind")}>
          <span className="gr-tools-label">{say("tools.filter.kind")}</span>
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
          <Toggle
            pressed={query.unmapped === true}
            label={say("tools.filter.unmapped")}
            onPress={() => {
              onQuery(
                patched(query, {
                  unmapped: query.unmapped === true ? undefined : true,
                }),
              );
            }}
          />
        </div>
        <FilterBar
          filters={view.applied}
          onRemove={(name) => {
            onQuery(patched(query, { [name]: undefined }));
          }}
          onClear={() => {
            onQuery(
              patched(query, {
                tool: undefined,
                kind: undefined,
                unmapped: undefined,
              }),
            );
          }}
        />
      </div>
      <div data-region="counts">
        <Tile
          label={say("tools.tile.tools")}
          value={<Count value={view.counts.tools} />}
        />
        <Tile
          label={say("tools.tile.entries")}
          value={<Count value={view.counts.entries} />}
        />
        <Tile
          label={say("tools.tile.rules")}
          value={<Count value={view.counts.rules} />}
        />
        <Tile
          label={say("tools.tile.unmapped")}
          value={<Count value={view.counts.unmapped} />}
        />
      </div>
      <p role="status" data-region="matching">
        {view.applied.length === 0
          ? say("tools.counts.entries", { count: view.counts.entries })
          : say("tools.applied", {
              shown: format.number(view.counts.entries),
              total: format.number(
                view.tools.reduce((n, t) => n + t.entries, 0),
              ),
            })}
      </p>
      <div data-region="tools">
        <ListTable
          caption={say("tools.list.caption")}
          columns={[
            { key: "tool", label: say("tools.col.tool") },
            { key: "version", label: say("tools.col.version") },
            { key: "adapter", label: say("tools.col.adapter") },
            { key: "entries", label: say("tools.col.entries"), numeric: true },
            { key: "rules", label: say("tools.col.rules"), numeric: true },
            { key: "standing", label: say("tools.col.standing") },
          ]}
          rows={rows}
        />
      </div>
      <div data-region="groups">
        {view.groups.length === 0 ? (
          <EmptyFrame title={say("tools.nothing-left")} />
        ) : (
          view.groups.map((group) => (
            <GroupView key={group.key} group={group} grouping={view.grouping} />
          ))
        )}
      </div>
      {view.open === null ? null : (
        <Panel
          tool={view.open}
          onClose={() => {
            onQuery(patched(query, { open: undefined }));
          }}
        />
      )}
    </>
  );
}

/** Draws one of the six states of the tools region: every failure offers Retry. */
export function ToolsBody({
  state,
  query,
  onQuery,
  onRetry,
}: ToolsBodyProps): ReactElement {
  return (
    <section data-view="tools" data-state={state.state}>
      <h1>{say("tools.heading")}</h1>
      {state.state === "loading" ? (
        <p role="status">{say("tools.loading")}</p>
      ) : state.state === "empty" ? (
        <EmptyFrame title={say("tools.empty")}>
          {say("tools.empty.hint")}
        </EmptyFrame>
      ) : state.state === "error" || state.state === "disconnected" ? (
        <ErrorFrame message={say("tools.error")} onRetry={onRetry} />
      ) : (
        <>
          {state.state === "stale" ? (
            <Banner tone="warning">
              {say("tools.stale", { age: format.duration(state.ageMs) })}
            </Banner>
          ) : null}
          <p className="gr-tools-intro">{say("tools.intro")}</p>
          <Ready view={state.value} query={query} onQuery={onQuery} />
        </>
      )}
    </section>
  );
}

export type ToolsPageProps = Readonly<{
  search: ToolsSearch;
  params?: Readonly<Record<string, string>>;
  onSearch?: (next: ToolsSearch) => void;
}>;

/** The one orchestrator: the address in, the loaded bundle in, plain values out. */
export function ToolsPage({ search, onSearch }: ToolsPageProps): ReactElement {
  const client = useQueryClient();
  // The shell has loaded the bundle before it draws a page; this only reads it.
  const snapshot = useQuery<Loaded<BundleValue>>({
    queryKey: keys.bundle(),
    queryFn: skipToken,
    ...freshness,
  });
  const region = bundleRegion(snapshot, snapshot.dataUpdatedAt);
  const state: DataState<ToolsView> =
    region.state === "ready" || region.state === "stale"
      ? { ...region, value: toolsView(region.value, search) }
      : region;
  return (
    <ToolsBody
      state={state}
      query={search}
      onQuery={(next) => {
        const { unmapped, ...rest } = next;
        onSearch?.(unmapped === true ? { ...rest, unmapped } : rest);
      }}
      onRetry={() => {
        void retryLoad(client, "bundle.json");
      }}
    />
  );
}
