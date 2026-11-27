// The relations view: the graph one hop around
// a rule, decision, source, binding, tool entry, part or term, as the view's own
// SVG, and the same connections as a list, which is what assistive technology
// reads and what a keyboard walks: each entity is a link to the graph around it,
// and each has a control that adds one more hop around it. A node with more
// neighbours than the cap shows an "N more" count in its place, never a frozen
// page. Which nodes are expanded is in the address (`?expanded=`), so a reload
// finds the same graph. `RelationsRegion` is plain props: it imports no data
// layer, so the element and the iframe mount it with a bundle in hand
// (react.components.1). `RelationsPage` is the one orchestrator that asks the
// cache. Every word is the message module's.
import { Button } from "@base-ui/react/button";
import { type BundleDocument } from "@gragas/contract";
import { skipToken, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ReactElement } from "react";
import * as z from "zod";
import {
  destinationPath,
  keys,
  relationKinds,
  retryLoad,
  type DataState,
  type Loaded,
  type RelationKind,
} from "../../data/index.ts";
import { createFormatter } from "../../formatter/index.ts";
import {
  createSay,
  missingFrame,
  noRowsFrame,
  staleBanner,
} from "../../messages/index.ts";
import { Banner, EmptyFrame, ErrorFrame, Link } from "../../parts/index.ts";
import { bundleRegion } from "../../selectors/index.ts";
import {
  relationsView,
  type RelationNode,
  type RelationsGraph,
} from "./selectors.ts";
import "../../parts/base.css";
import "./relations.css";

type Bundle = z.output<typeof BundleDocument>;

export const relationsSearchSchema = z.strictObject({
  /** Node keys (`kind:id`) with one more hop drawn around each. */
  expanded: z.array(z.string()).optional(),
});

export type RelationsSearch = z.output<typeof relationsSearchSchema>;

const say = createSay("en");
const format = createFormatter();

export const states = [
  "default",
  "loading",
  "empty",
  "error",
  "phone390",
  "stale",
] as const;

const kindOf = (value: string): RelationKind | undefined =>
  relationKinds.find((k) => k === value);

const hrefOf = (node: RelationNode): string =>
  `#${destinationPath({ route: "relations", kind: node.kind, id: node.id })}`;

const width = 640;
const height = 400;
const viewBox = [0, 0, width, height].join(" ");

/** Where each node sits: the centre in the middle, the rest round an ellipse. */
function place(
  graph: RelationsGraph,
): ReadonlyMap<string, { readonly x: number; readonly y: number }> {
  const spots = new Map<string, { readonly x: number; readonly y: number }>();
  spots.set(graph.center.key, { x: width / 2, y: height / 2 });
  const others = graph.nodes.filter((n) => n.key !== graph.center.key);
  others.forEach((node, i) => {
    const angle = (2 * Math.PI * i) / Math.max(1, others.length) - Math.PI / 2;
    spots.set(node.key, {
      x: Math.round(width / 2 + (width / 2 - 70) * Math.cos(angle)),
      y: Math.round(height / 2 + (height / 2 - 40) * Math.sin(angle)),
    });
  });
  return spots;
}

const clip = (text: string): string =>
  text.length > 20 ? `${text.slice(0, 19)}…` : text;

function Drawing({ graph }: Readonly<{ graph: RelationsGraph }>): ReactElement {
  const spots = place(graph);
  return (
    <svg
      className="gr-relations-svg"
      data-part="relations-graph"
      viewBox={viewBox}
      role="img"
      aria-label={say("relations.graph", { id: graph.center.id })}
    >
      {graph.edges.map((e) => {
        const from = spots.get(e.from);
        const to = spots.get(e.to);
        if (from === undefined || to === undefined) return null;
        return (
          <g
            key={`${e.from}|${e.label}|${e.to}`}
            data-part="relations-edge"
            data-from={e.from}
            data-label={e.label}
            data-to={e.to}
          >
            <line x1={from.x} y1={from.y} x2={to.x} y2={to.y} />
            <text x={(from.x + to.x) / 2} y={(from.y + to.y) / 2}>
              {say(`relations.edge.${e.label}`)}
            </text>
          </g>
        );
      })}
      {graph.nodes.map((n) => {
        const at = spots.get(n.key);
        if (at === undefined) return null;
        return (
          <g
            key={n.key}
            data-part="relations-node"
            data-key={n.key}
            data-center={n.key === graph.center.key ? "true" : undefined}
            data-known={n.known ? "true" : "false"}
          >
            <circle
              cx={at.x}
              cy={at.y}
              r={n.key === graph.center.key ? 12 : 8}
            />
            <text x={at.x} y={at.y + 24}>
              {clip(n.label)}
            </text>
          </g>
        );
      })}
    </svg>
  );
}

function Entity({
  node,
  graph,
  expanded,
  onExpand,
}: Readonly<{
  node: RelationNode;
  graph: RelationsGraph;
  expanded: readonly string[];
  onExpand: ((keys: readonly string[]) => void) | undefined;
}>): ReactElement {
  const open = expanded.includes(node.key);
  const more = graph.more[node.key] ?? 0;
  return (
    <li data-part="relations-entity" data-key={node.key}>
      <span className="gr-relations-kind">
        {say(`relations.kind.${node.kind}`)}
      </span>{" "}
      <Link href={hrefOf(node)}>{node.id}</Link>
      {node.known ? null : (
        <span className="gr-relations-note">
          {" "}
          {say("relations.node.unheld")}
        </span>
      )}
      {more > 0 ? (
        <span className="gr-relations-note" data-part="relations-more">
          {" "}
          {say("relations.node.more", { count: more })}
        </span>
      ) : null}
      {onExpand === undefined ? null : (
        <Button
          className="gr-button gr-target"
          aria-expanded={open}
          onClick={() => {
            onExpand(
              open
                ? expanded.filter((k) => k !== node.key)
                : [...expanded, node.key],
            );
          }}
        >
          {say(open ? "relations.node.collapse" : "relations.node.expand", {
            id: node.id,
          })}
        </Button>
      )}
    </li>
  );
}

function Content({
  bundle,
  kind,
  id,
  expanded,
  onExpand,
}: Readonly<{
  bundle: Bundle;
  kind: string;
  id: string;
  expanded: readonly string[];
  onExpand: ((keys: readonly string[]) => void) | undefined;
}>): ReactElement {
  const entity = kindOf(kind);
  if (entity === undefined) {
    return <EmptyFrame title={say("relations.bad-kind", { kind })} />;
  }
  const result = relationsView(bundle, entity, id, { expanded });
  if (result.kind === "missing") {
    return (
      <EmptyFrame
        title={say("relations.missing", {
          kind: say(`relations.kind.${result.entity}`),
          id: result.id,
        })}
      />
    );
  }
  const { graph } = result;
  return (
    <>
      <h2 data-part="relations-center">
        {say("relations.around", {
          kind: say(`relations.kind.${graph.center.kind}`),
          id: graph.center.id,
        })}
      </h2>
      <p data-part="relations-counts">
        {say("relations.counts", { count: graph.list.length })}
        {". "}
        {format.number(graph.nodes.length, 0)} /{" "}
        {format.number(graph.list.length, 0)}
      </p>
      <Drawing graph={graph} />
      <section aria-labelledby="relations-list">
        <h2 id="relations-list">{say("relations.list")}</h2>
        <ul className="gr-relations-list" data-part="relations-list">
          {graph.list.map((row) => (
            <li
              key={`${row.from.key}|${row.label}|${row.to.key}`}
              data-part="relations-row"
              data-from={row.from.key}
              data-label={row.label}
              data-to={row.to.key}
            >
              <Link href={hrefOf(row.from)}>{row.from.id}</Link>{" "}
              {say(`relations.edge.${row.label}`)}{" "}
              <Link href={hrefOf(row.to)}>{row.to.id}</Link>
            </li>
          ))}
        </ul>
      </section>
      <section aria-labelledby="relations-nodes">
        <h2 id="relations-nodes">{say("relations.nodes")}</h2>
        <ul className="gr-relations-list">
          {graph.nodes.map((node) => (
            <Entity
              key={node.key}
              node={node}
              graph={graph}
              expanded={expanded}
              onExpand={onExpand}
            />
          ))}
        </ul>
      </section>
    </>
  );
}

export type RelationsRegionProps = Readonly<{
  region: DataState<Bundle>;
  /** The entity the address names: its kind and its id. */
  kind: string;
  id: string;
  /** The node keys with one more hop; none when the address names none. */
  expanded?: readonly string[] | undefined;
  /** Writes the new set of expanded keys; absent where nothing owns an address. */
  onExpand?: ((keys: readonly string[]) => void) | undefined;
  onRetry: () => void;
}>;

export function RelationsRegion({
  region,
  kind,
  id,
  expanded = [],
  onExpand,
  onRetry,
}: RelationsRegionProps): ReactElement {
  return (
    <section data-view="relations" data-state={region.state}>
      <h1>{say("shell.view.relations")}</h1>
      <p className="gr-relations-question">{say("relations.question")}</p>
      {region.state === "loading" ? (
        <p role="status">{say("shell.loading")}</p>
      ) : null}
      {region.state === "empty" ? (
        <EmptyFrame
          title={
            region.why === "no-rows"
              ? say("relations.no-entity")
              : missingFrame(region.file)
          }
        >
          {region.why === "no-rows" ? (
            <>
              <p>{noRowsFrame(region.file)}</p>
              <p>{say("relations.handbook")}</p>
            </>
          ) : null}
        </EmptyFrame>
      ) : null}
      {region.state === "error" || region.state === "disconnected" ? (
        <ErrorFrame
          message={say("shell.data.failed", { file: region.file })}
          onRetry={onRetry}
        />
      ) : null}
      {region.state === "stale" ? (
        <Banner tone="warning">{staleBanner(region.ageMs)}</Banner>
      ) : null}
      {region.state === "ready" || region.state === "stale" ? (
        <Content
          bundle={region.value}
          kind={kind}
          id={id}
          expanded={expanded}
          onExpand={onExpand}
        />
      ) : null}
    </section>
  );
}

/** The page: reads the one cached bundle the shell loaded and hands it down. */
export function RelationsPage({
  search,
  params,
  onSearch,
}: Readonly<{
  search: RelationsSearch;
  params: Readonly<Record<string, string>>;
  onSearch?: ((next: RelationsSearch) => void) | undefined;
}>): ReactElement {
  const client = useQueryClient();
  const bundle = useQuery<Loaded<Bundle>>({
    queryKey: keys.bundle(),
    queryFn: skipToken,
    staleTime: Infinity,
  });
  return (
    <RelationsRegion
      region={bundleRegion(bundle, bundle.dataUpdatedAt)}
      kind={params["kind"] ?? ""}
      id={params["id"] ?? ""}
      expanded={search.expanded}
      onExpand={
        onSearch === undefined
          ? undefined
          : (next) => {
              onSearch(next.length === 0 ? {} : { expanded: [...next] });
            }
      }
      onRetry={() => {
        void retryLoad(client, "bundle.json");
      }}
    />
  );
}
