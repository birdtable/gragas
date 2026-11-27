// The attention view: the rules that need a person now, grouped by
// why, in eight kinds, each item with its rule, date and owner; the rulebook's
// open conflicts and stale renderings follow as two further lists. The day is
// read once from the clock when the page draws and is shown (S55, web.content.4);
// "high override rate" needs two batches the bundle never carries, so it says
// "override rate: no batches given". The kind filter is in the URL
// (`#/attention?kind=overdue`), a link a keyboard reaches. The view is the
// rulebook's, never one project's. `AttentionRegion` is plain props and imports
// no data layer (react.components.1); `AttentionPage` is the one orchestrator.
import type { BundleDocument } from "@gragas/contract";
import { skipToken, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ReactElement } from "react";
import * as z from "zod";
import {
  dayStart,
  destinationPath,
  keys,
  retryLoad,
  today as clockToday,
  type DataState,
  type Loaded,
} from "../../data/index.ts";
import { createFormatter } from "../../formatter/index.ts";
import {
  createSay,
  missingFrame,
  noRowsFrame,
  staleBanner,
} from "../../messages/index.ts";
import {
  Banner,
  Dash,
  EmptyFrame,
  ErrorFrame,
  Link,
} from "../../parts/index.ts";
import { bundleRegion } from "../../selectors/index.ts";
import {
  attentionKinds,
  attentionView,
  type AttentionGroup,
  type AttentionItem,
  type AttentionView,
  type Filters,
} from "./selectors.ts";
import "../../parts/base.css";
import "./attention.css";

type Bundle = z.output<typeof BundleDocument>;

export const attentionSearchSchema = z.strictObject({
  kind: z.enum(attentionKinds).optional(),
});

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

const hrefFor = (kind: Filters["kind"]): string =>
  kind === undefined ? "#/attention" : `#/attention?kind=${kind}`;

const dayText = (day: string): string => format.date(dayStart(day));

function Kinds({ filters }: Readonly<{ filters: Filters }>): ReactElement {
  return (
    <nav data-part="attention-filter" aria-label={say("attention.filter.kind")}>
      <p className="gr-attention-mark">{say("attention.filter.kind")}</p>
      <ul className="gr-attention-options">
        {[undefined, ...attentionKinds].map((kind) => (
          <li key={kind ?? "all"}>
            <Link href={hrefFor(kind)}>
              <span aria-current={filters.kind === kind ? "true" : undefined}>
                {kind === undefined
                  ? say("attention.filter.all")
                  : say(`attention.kind.${kind}`)}
              </span>
            </Link>
          </li>
        ))}
      </ul>
      {filters.kind === undefined ? null : (
        <p>
          <Link href="#/attention">{say("attention.filter.clear")}</Link>
        </p>
      )}
    </nav>
  );
}

function Why({ item }: Readonly<{ item: AttentionItem }>): ReactElement {
  if (item.kind === "missing") {
    return (
      <p>
        {item.missing.length === 0
          ? null
          : say("attention.why.missing", {
              what: format.list(
                item.missing.map((m) => say(`attention.missing.${m}`)),
              ),
            })}{" "}
        {item.decision === null ? null : (
          <span data-part="attention-decision">
            {say("attention.decision.label")}{" "}
            <Dash reason={say(item.decision.reason)} />
          </span>
        )}
      </p>
    );
  }
  if (item.kind === "trial-overrun" && item.trial !== null) {
    return (
      <p>
        {say("attention.why.trial-overrun", {
          shipped: format.number(item.trial.shipped, 0),
          allowed: format.number(item.trial.allowed, 0),
        })}
      </p>
    );
  }
  if (item.kind === "review-held") {
    return (
      <p>
        {say("attention.why.review-held", {
          path: item.path ?? "",
          shown: item.shown ?? "",
        })}
      </p>
    );
  }
  return <p>{say(`attention.why.${item.kind}`)}</p>;
}

function Item({ item }: Readonly<{ item: AttentionItem }>): ReactElement {
  const dateLabel =
    item.kind === "overdue"
      ? say("attention.item.date.review-by")
      : item.kind === "past-zero-by"
        ? say("attention.item.date.zero-by")
        : say("attention.item.date");
  return (
    <li data-part="attention-item" data-kind={item.kind}>
      <dl className="gr-attention-fields">
        <dt>{say("attention.item.rule")}</dt>
        <dd>
          <Link href={`#${destinationPath({ route: "rule", id: item.rule })}`}>
            {item.rule}
          </Link>
        </dd>
        <dt>{say("attention.item.why")}</dt>
        <dd>
          <Why item={item} />
        </dd>
        <dt>{dateLabel}</dt>
        <dd data-part="attention-date">
          {item.date.kind === "dash" ? (
            <Dash reason={say(item.date.reason)} />
          ) : (
            dayText(item.date.value)
          )}
        </dd>
        <dt>{say("attention.item.owner")}</dt>
        <dd data-part="attention-owner">
          {item.owner.kind === "dash" ? (
            <Dash reason={say(item.owner.reason)} />
          ) : (
            item.owner.value
          )}
        </dd>
      </dl>
    </li>
  );
}

function Group({ group }: Readonly<{ group: AttentionGroup }>): ReactElement {
  return (
    <li data-part="attention-group" data-kind={group.kind}>
      <h2>{say(`attention.kind.${group.kind}`)}</h2>
      {group.needsBatches ? (
        <>
          <p data-part="attention-override-rate">
            {say("attention.override-rate.none")}
          </p>
          <p className="gr-attention-mark">
            {say("attention.override-rate.why")}
          </p>
        </>
      ) : (
        <>
          <p className="gr-attention-mark">
            {group.items.length === 0
              ? say("attention.group.empty")
              : say("attention.group.count", { count: group.items.length })}
          </p>
          <ul className="gr-attention-items">
            {group.items.map((item) => (
              <Item key={`${item.kind}/${item.rule}`} item={item} />
            ))}
          </ul>
        </>
      )}
    </li>
  );
}

function Further({ view }: Readonly<{ view: AttentionView }>): ReactElement {
  return (
    <div className="gr-attention-further">
      <section data-part="attention-further" data-kind="conflicts">
        <h2>{say("attention.conflicts")}</h2>
        {view.conflicts.length === 0 ? (
          <p>{say("attention.conflicts.none")}</p>
        ) : (
          <ul className="gr-attention-items">
            {view.conflicts.map((c) => (
              <li
                key={`${c.entity}/${c.message}`}
                data-part="attention-conflict"
              >
                <p>{c.message}</p>
                <p className="gr-attention-mark">
                  {say("attention.conflicts.fix")}:{" "}
                  {c.fix.kind === "dash" ? (
                    <Dash reason={say(c.fix.reason)} />
                  ) : (
                    c.fix.value
                  )}
                </p>
                {c.rule.kind === "value" ? (
                  <Link
                    href={`#${destinationPath({ route: "rule", id: c.rule.value })}`}
                  >
                    {c.rule.value}
                  </Link>
                ) : null}
              </li>
            ))}
          </ul>
        )}
      </section>
      <section data-part="attention-further" data-kind="renderings">
        <h2>{say("attention.renderings")}</h2>
        {view.staleRenderings.length === 0 ? (
          <p>{say("attention.renderings.none")}</p>
        ) : (
          <ul className="gr-attention-items">
            {view.staleRenderings.map((s) => (
              <li key={`${s.rule}/${s.path}`} data-part="attention-rendering">
                <Link
                  href={`#${destinationPath({ route: "rule", id: s.rule })}`}
                >
                  {s.rule}
                </Link>{" "}
                {say("attention.renderings.row", {
                  path: s.path,
                  shown: s.shown,
                  now: s.now,
                })}
              </li>
            ))}
          </ul>
        )}
      </section>
    </div>
  );
}

function Content({
  bundle,
  today,
  filters,
}: Readonly<{
  bundle: Bundle;
  today: string;
  filters: Filters;
}>): ReactElement {
  const view = attentionView(bundle, today, filters);
  const shown = view.shownGroups.reduce((n, g) => n + g.items.length, 0);
  return (
    <>
      <p data-part="attention-today">
        {say("attention.today", { date: dayText(view.today) })}
      </p>
      {view.allProposed ? (
        <p data-part="attention-all-proposed" role="note">
          {say("attention.all-proposed")}
        </p>
      ) : null}
      <p data-part="attention-counts">
        {view.itemCount === 0
          ? say("attention.nothing")
          : say("attention.count", {
              count: view.ruleCount,
              items: format.number(view.itemCount, 0),
            })}
        {". "}
        {say("attention.filter.shown", { count: shown })}
      </p>
      <Kinds filters={filters} />
      <ul
        className="gr-attention-groups"
        aria-label={say("shell.view.attention")}
      >
        {view.shownGroups.map((group) => (
          <Group key={group.kind} group={group} />
        ))}
      </ul>
      <Further view={view} />
    </>
  );
}

export type AttentionRegionProps = Readonly<{
  region: DataState<Bundle>;
  /** The day the dates are judged against, `YYYY-MM-DD`: the clock's, read by the page. */
  today: string;
  /** The filter in the URL; none when absent. */
  filters?: Filters | undefined;
  onRetry: () => void;
}>;

export function AttentionRegion({
  region,
  today,
  filters,
  onRetry,
}: AttentionRegionProps): ReactElement {
  return (
    <section data-view="attention" data-state={region.state}>
      <h1>{say("shell.view.attention")}</h1>
      <p className="gr-attention-question">{say("attention.question")}</p>
      <p className="gr-attention-nothing">{say("attention.handbook")}</p>
      {region.state === "loading" ? (
        <p role="status">{say("shell.loading")}</p>
      ) : null}
      {region.state === "empty" ? (
        <EmptyFrame
          title={
            region.why === "no-rows"
              ? say("attention.nothing")
              : missingFrame(region.file)
          }
        >
          {region.why === "no-rows" ? <p>{noRowsFrame(region.file)}</p> : null}
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
        <Content bundle={region.value} today={today} filters={filters ?? {}} />
      ) : null}
    </section>
  );
}

/** The page: reads the one cached bundle and the clock, and hands both down. */
export function AttentionPage({
  search,
}: Readonly<{ search: z.output<typeof attentionSearchSchema> }>): ReactElement {
  const client = useQueryClient();
  const bundle = useQuery<Loaded<Bundle>>({
    queryKey: keys.bundle(),
    queryFn: skipToken,
    staleTime: Infinity,
  });
  return (
    <AttentionRegion
      region={bundleRegion(bundle, bundle.dataUpdatedAt)}
      today={clockToday()}
      filters={search}
      onRetry={() => {
        void retryLoad(client, "bundle.json");
      }}
    />
  );
}
