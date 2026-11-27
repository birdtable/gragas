// The changelog view: the releases newest first, each with its bump
// kind and date, and one row per change with its consumer action, its remedy and
// its diff link. A rule that a change names links to its rule view, a retired one
// too. The filters are in the URL (`#/changelog?change=retired&action=none`), so
// a reload finds the same state and each is a link a keyboard reaches. Nothing
// is applied here: upgrade in the project applies a change after a yes.
// `ChangelogRegion` is plain props and imports no data layer, so the element and
// the iframe mount it with a bundle in hand (react.components.1);
// `ChangelogPage` is the one orchestrator that asks the cache. Every word is the
// message module's, every number and date the formatter's.
import type { BundleDocument } from "@gragas/contract";
import { skipToken, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ReactElement } from "react";
import * as z from "zod";
import {
  destinationPath,
  keys,
  retryLoad,
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
  changeKinds,
  changelogView,
  type ActionFilter,
  type ChangeRow,
  type ChangelogView,
  type Filters,
  type ReleaseBlock,
} from "./selectors.ts";
import "../../parts/base.css";
import "./changelog.css";

type Bundle = z.output<typeof BundleDocument>;

export const changelogSearchSchema = z.strictObject({
  change: z.enum(changeKinds).optional(),
  action: z.enum(["some", "none"]).optional(),
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

const releaseId = (version: string): string => `release-${version}`;

function hrefFor(filters: Filters): string {
  const pairs = [
    ["change", filters.change],
    ["action", filters.action],
  ].flatMap(([name, value]) =>
    value === undefined ? [] : [`${name ?? ""}=${encodeURIComponent(value)}`],
  );
  return pairs.length === 0 ? "#/changelog" : `#/changelog?${pairs.join("&")}`;
}

function Options({
  label,
  options,
}: Readonly<{
  label: string;
  options: readonly Readonly<{
    key: string;
    text: string;
    href: string;
    current: boolean;
  }>[];
}>): ReactElement {
  return (
    <nav data-part="changelog-filter" aria-label={label}>
      <p className="gr-changelog-mark">{label}</p>
      <ul className="gr-changelog-options">
        {options.map((o) => (
          <li key={o.key}>
            <Link href={o.href}>
              <span aria-current={o.current ? "true" : undefined}>
                {o.text}
              </span>
            </Link>
          </li>
        ))}
      </ul>
    </nav>
  );
}

function FilterGroups({
  view,
  filters,
}: Readonly<{ view: ChangelogView; filters: Filters }>): ReactElement {
  const actions: readonly (ActionFilter | undefined)[] = [
    undefined,
    "some",
    "none",
  ];
  return (
    <div className="gr-changelog-filters">
      <Options
        label={say("changelog.filter.change")}
        options={[undefined, ...view.kinds].map((kind) => ({
          key: kind ?? "all",
          text:
            kind === undefined
              ? say("changelog.filter.all")
              : say(`changelog.change.${kind}`),
          href: hrefFor({ ...filters, change: kind }),
          current: filters.change === kind,
        }))}
      />
      <Options
        label={say("changelog.filter.action")}
        options={actions.map((action) => ({
          key: action ?? "all",
          text:
            action === undefined
              ? say("changelog.filter.all")
              : say(`changelog.filter.action.${action}`),
          href: hrefFor({ ...filters, action }),
          current: filters.action === action,
        }))}
      />
      {filters.change === undefined && filters.action === undefined ? null : (
        <p>
          <Link href="#/changelog">{say("changelog.filter.clear")}</Link>
        </p>
      )}
    </div>
  );
}

function EntityLink({ row }: Readonly<{ row: ChangeRow }>): ReactElement {
  const label = (
    <>
      <span className="gr-changelog-mark">
        {say(`changelog.kind.${row.entityKind}`)}{" "}
      </span>
      {row.id}
    </>
  );
  if (row.ruleView) {
    return (
      <Link href={`#${destinationPath({ route: "rule", id: row.id })}`}>
        {label}
      </Link>
    );
  }
  if (row.entityKind === "term") {
    return <Link href="#/terms">{label}</Link>;
  }
  return <span>{label}</span>;
}

function Change({ row }: Readonly<{ row: ChangeRow }>): ReactElement {
  return (
    <li data-part="changelog-change" data-change={row.change}>
      <dl className="gr-changelog-fields">
        <dt>{say("changelog.entity")}</dt>
        <dd>
          <EntityLink row={row} />
        </dd>
        <dt>{say("changelog.change")}</dt>
        <dd>
          <span data-part="changelog-kind">
            <span aria-hidden="true">{row.leaves ? "× " : "· "}</span>
            {say(`changelog.change.${row.change}`)}
          </span>
          {row.leaves && row.ruleView ? (
            <p className="gr-changelog-mark" role="note">
              {say("changelog.retired")}
            </p>
          ) : null}
        </dd>
        <dt>{say("changelog.sentence")}</dt>
        <dd>
          {row.sentence.kind === "dash" ? (
            <Dash reason={say(row.sentence.reason)} />
          ) : (
            <p>{row.sentence.value}</p>
          )}
        </dd>
        <dt>{say("changelog.action")}</dt>
        <dd data-part="changelog-action" data-action={row.action.kind}>
          {row.action.kind === "none" ? (
            <p>{say("changelog.action.none")}</p>
          ) : (
            <p>{row.action.text}</p>
          )}
        </dd>
        <dt>{say("changelog.remedy")}</dt>
        <dd>
          {row.remedy.kind === "dash" ? (
            <Dash reason={say(row.remedy.reason)} />
          ) : (
            <p>{row.remedy.value}</p>
          )}
        </dd>
        <dt>{say("changelog.diff")}</dt>
        <dd data-part="changelog-diff">
          {row.diff === null ? (
            <Dash reason={say("absent.none-recorded")} />
          ) : (
            <Link href={row.diff} external>
              {row.entityKind === "checker-code"
                ? say("changelog.diff.checker-code")
                : say("changelog.diff.link")}
            </Link>
          )}
        </dd>
      </dl>
    </li>
  );
}

function Release({ block }: Readonly<{ block: ReleaseBlock }>): ReactElement {
  return (
    <li data-part="changelog-release" id={releaseId(block.version)}>
      <h2>{say("changelog.release", { version: block.version })}</h2>
      <p data-part="changelog-bump">
        {block.bump.kind === "dash" ? (
          <Dash reason={say(block.bump.reason)} />
        ) : (
          say("changelog.release.bump", {
            bump: say(`changelog.bump.${block.bump.value}`),
          })
        )}
        {block.date.kind === "value"
          ? `, ${say("changelog.release.date", {
              date: format.date(Date.parse(block.date.value)),
            })}`
          : null}
        {". "}
        {say("changelog.release.changes", { count: block.total })}
      </p>
      <ul className="gr-changelog-changes">
        {block.rows.map((row) => (
          <Change key={`${row.entity}/${row.change}`} row={row} />
        ))}
      </ul>
    </li>
  );
}

function Content({
  bundle,
  filters,
}: Readonly<{ bundle: Bundle; filters: Filters }>): ReactElement {
  const everything = changelogView(bundle);
  if (everything.changeCount === 0) {
    return (
      <EmptyFrame title={say("changelog.no-changes")}>
        <p>{say("changelog.handbook")}</p>
      </EmptyFrame>
    );
  }
  const view = changelogView(bundle, filters);
  return (
    <>
      <p data-part="changelog-counts">
        {say("changelog.count", {
          count: view.releaseCount,
          changes: format.number(view.changeCount, 0),
        })}
        {". "}
        {say("changelog.filter.shown", {
          count: view.shown,
          total: format.number(view.changeCount, 0),
        })}
      </p>
      <FilterGroups view={everything} filters={filters} />
      {view.releases.length === 0 ? (
        <EmptyFrame title={say("changelog.no-match")}>
          <p>
            <Link href="#/changelog">{say("changelog.filter.clear")}</Link>
          </p>
        </EmptyFrame>
      ) : (
        <ul
          className="gr-changelog-releases"
          aria-label={say("shell.view.changelog")}
        >
          {view.releases.map((block) => (
            <Release key={block.version} block={block} />
          ))}
        </ul>
      )}
    </>
  );
}

export type ChangelogRegionProps = Readonly<{
  region: DataState<Bundle>;
  /** The filters in the URL; none when absent. */
  filters?: Filters | undefined;
  onRetry: () => void;
}>;

export function ChangelogRegion({
  region,
  filters,
  onRetry,
}: ChangelogRegionProps): ReactElement {
  return (
    <section data-view="changelog" data-state={region.state}>
      <h1>{say("shell.view.changelog")}</h1>
      <p className="gr-changelog-question">{say("changelog.question")}</p>
      <p className="gr-changelog-nothing" data-part="changelog-nothing-applied">
        {say("changelog.nothing-applied")}
      </p>
      {region.state === "loading" ? (
        <p role="status">{say("shell.loading")}</p>
      ) : null}
      {region.state === "empty" ? (
        <EmptyFrame
          title={
            region.why === "no-rows"
              ? say("changelog.no-changes")
              : missingFrame(region.file)
          }
        >
          {region.why === "no-rows" ? (
            <>
              <p>{noRowsFrame(region.file)}</p>
              <p>{say("changelog.handbook")}</p>
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
        <Content bundle={region.value} filters={filters ?? {}} />
      ) : null}
    </section>
  );
}

/** The page: reads the one cached bundle the shell loaded and hands it down. */
export function ChangelogPage({
  search,
}: Readonly<{ search: z.output<typeof changelogSearchSchema> }>): ReactElement {
  const client = useQueryClient();
  const bundle = useQuery<Loaded<Bundle>>({
    queryKey: keys.bundle(),
    queryFn: skipToken,
    staleTime: Infinity,
  });
  return (
    <ChangelogRegion
      region={bundleRegion(bundle, bundle.dataUpdatedAt)}
      filters={search}
      onRetry={() => {
        void retryLoad(client, "bundle.json");
      }}
    />
  );
}
