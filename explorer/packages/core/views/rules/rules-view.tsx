// The rules view: "Which rules apply to a program like mine, and
// what holds each?" It takes the bundle's data region, the filters in force and
// callbacks as plain props and imports no data layer (react.components.1), so
// the standalone packaging, an embed with host-held filters and a story render
// the same component with no provider. The filters live in the address, never
// here: a change goes out through `onFilters`.
import { Input } from "@base-ui/react/input";
import { Select } from "@base-ui/react/select";
import type { BundleDocument } from "@gragas/contract";
import type { ReactElement, ReactNode } from "react";
import type * as z from "zod";
import type { DataState, LoadFailure } from "../../data/index.js";
import {
  markSymbols,
  missingFrame,
  noRowsFrame,
  notReadyFrame,
  staleBanner,
} from "../../messages/index.ts";
import { Banner } from "../../parts/banner/banner.tsx";
import { Dash } from "../../parts/dash/dash.tsx";
import { EmptyFrame } from "../../parts/empty-frame/empty-frame.tsx";
import { ErrorFrame } from "../../parts/error-frame/error-frame.tsx";
import { FilterBar } from "../../parts/filter-bar/filter-bar.tsx";
import { ListTable, type Row } from "../../parts/list-table/list-table.tsx";
import { say } from "../../parts/say.ts";
import { StatusMark } from "../../parts/status-mark/status-mark.tsx";
import "../../parts/base.css";
import "./rules-view.css";
import {
  applied,
  filterNames,
  searchNames,
  without,
  type FilterName,
  type Filters,
  type SearchName,
} from "./filters.ts";
import {
  countsFor,
  factsFor,
  filterRules,
  groupByPart,
  type Rule,
  type RuleFacts,
} from "./selectors.ts";

type Bundle = z.output<typeof BundleDocument>;

/** The six states of a data region, each drawn (web.states.1). */
export const states = [
  "ready",
  "loading",
  "empty",
  "error",
  "stale",
  "disconnected",
] as const;

export type RulesViewProps = Readonly<{
  region: DataState<Bundle>;
  filters: Filters;
  onFilters: (next: Filters) => void;
  onRetry: () => void;
  /** Where a rule's own page is; the standalone's by default, the portal's in an embed. */
  ruleHref?: (id: string) => string;
}>;

const ruleAddress = (id: string): string => `#/rules/${id}`;

function failure(cause: LoadFailure): string {
  if (cause.kind === "missing") return missingFrame(cause.file);
  if (cause.kind === "not-ready") {
    return notReadyFrame(
      cause.file,
      cause.documentVersion,
      cause.readerVersion,
    );
  }
  return say("shell.data.failed", { file: cause.file });
}

function isIn<T extends string>(list: readonly T[], value: string): value is T {
  return list.some((item) => item === value);
}

const standingWords = [
  "proposed",
  "enforced",
  "reviewed",
  "planned",
  "advisory",
  "retired",
] as const;
const markWords = ["trial", "blocked", "unproven"] as const;
const statusWords = ["proposed", "trial", "active", "retired"] as const;

/** A value of a facet in words: a standing, a mark or a status says its word. */
function wordFor(name: FilterName, value: string): string {
  if (name === "standing" && isIn(standingWords, value)) {
    return say(`standing.${value}`);
  }
  if (name === "mark" && isIn(markWords, value)) return say(`mark.${value}`);
  if (name === "status" && isIn(statusWords, value)) {
    return say(`status.${value}`);
  }
  return value;
}

const facetLabel = (name: SearchName): string =>
  name === "q" ? say("rules.search") : say(`rules.facet.${name}`);

function Facet({
  name,
  values,
  value,
  onChange,
}: Readonly<{
  name: FilterName;
  values: ReadonlyMap<string, number>;
  value: string | undefined;
  onChange: (next: string | undefined) => void;
}>): ReactElement {
  const items = [
    { value: "", label: say("rules.facet.any") },
    ...[...values].map(([v, count]) => ({
      value: v,
      label: say("rules.facet.option", {
        value: wordFor(name, v),
        count: String(count),
      }),
    })),
  ];
  return (
    <div data-part="facet" data-facet={name}>
      <span className="gr-facet-label" id={`gr-facet-${name}`}>
        {facetLabel(name)}
      </span>
      <Select.Root
        items={items}
        value={value ?? ""}
        onValueChange={(next) => {
          onChange(next === null || next === "" ? undefined : next);
        }}
      >
        <Select.Trigger
          className="gr-facet-trigger gr-target"
          aria-labelledby={`gr-facet-${name}`}
        >
          <Select.Value />
        </Select.Trigger>
        <Select.Portal>
          <Select.Positioner className="gr-facet-positioner" sideOffset={4}>
            <Select.Popup className="gr-facet-popup">
              <Select.List>
                {items.map((item) => (
                  <Select.Item
                    key={item.value}
                    value={item.value}
                    className="gr-facet-item gr-target"
                  >
                    <Select.ItemText>{item.label}</Select.ItemText>
                  </Select.Item>
                ))}
              </Select.List>
            </Select.Popup>
          </Select.Positioner>
        </Select.Portal>
      </Select.Root>
    </div>
  );
}

function Chip({
  kind,
  text,
}: Readonly<{ kind: string; text: string }>): ReactElement {
  return (
    <span data-part="rule-chip" data-chip={kind}>
      {text}
    </span>
  );
}

function Standing({ rule }: Readonly<{ rule: Rule }>): ReactElement {
  const [first, ...rest] = rule.marks;
  return (
    <span className="gr-rules-standing">
      {first === undefined ? (
        <StatusMark standing={rule.standing} />
      ) : (
        <StatusMark standing={rule.standing} mark={first} />
      )}
      {rest.map((mark) => (
        <span key={mark} data-part="rule-mark" data-mark={mark}>
          <span aria-hidden="true">{markSymbols[mark]}</span>{" "}
          <span>{say(`mark.${mark}`)}</span>
        </span>
      ))}
    </span>
  );
}

function holdsCell(facts: RuleFacts): ReactNode {
  if (facts.tools.length === 0) return <Dash reason={say("rules.none.tool")} />;
  const tools = facts.tools.map((tool) => (
    <Chip key={tool} kind="tool" text={say("rules.chip.tool", { tool })} />
  ));
  const tier =
    facts.tier === null ? null : (
      <Chip
        kind="tier"
        text={say("rules.chip.tier", { rung: String(facts.tier) })}
      />
    );
  const fix = facts.fix ? (
    <Chip kind="fix" text={say("rules.chip.fix")} />
  ) : null;
  return (
    <span className="gr-rules-chips">
      {tools}
      {tier}
      {fix}
    </span>
  );
}

/** One table row: each cell is built first, so no word of it is a literal. */
function rowOf(
  rule: Rule,
  bundle: Bundle,
  ruleHref: (id: string) => string,
): Row {
  const facts = factsFor(rule, bundle.bindings, bundle.structure);
  const link = <a href={ruleHref(rule.id)}>{rule.id}</a>;
  const standing = <Standing rule={rule} />;
  const owner =
    rule.owner === "" ? <Dash reason={say("rules.none.owner")} /> : rule.owner;
  const layer = facts.layer ?? <Dash reason={say("rules.none.layer")} />;
  return {
    id: rule.id,
    cells: {
      rule: link,
      statement: rule.statement,
      strength: rule.strength,
      standing,
      owner,
      layer,
      holds: holdsCell(facts),
    },
  };
}

function Loaded({
  bundle,
  filters,
  onFilters,
  ruleHref,
}: Readonly<{
  bundle: Bundle;
  filters: Filters;
  onFilters: (next: Filters) => void;
  ruleHref: (id: string) => string;
}>): ReactElement {
  const { structure } = bundle;
  const counts = countsFor(bundle.rules, filters, structure.parts);
  const shown = filterRules(bundle.rules, filters, structure.parts);
  const groups = groupByPart(shown, structure.parts);
  const set = (name: SearchName, value: string | undefined): void => {
    onFilters({
      ...without(filters, name),
      ...(value === undefined ? {} : { [name]: value }),
    });
  };
  return (
    <>
      <div
        data-part="rules-filters"
        role="group"
        aria-label={say("rules.filters")}
      >
        <div className="gr-rules-search">
          <label htmlFor="gr-rules-q">{say("rules.search")}</label>
          <Input
            id="gr-rules-q"
            type="search"
            className="gr-rules-q gr-target"
            placeholder={say("rules.search.placeholder")}
            value={filters.q ?? ""}
            onValueChange={(next) => {
              set("q", next === "" ? undefined : next);
            }}
          />
        </div>
        {filterNames.map((name) => (
          <Facet
            key={name}
            name={name}
            values={counts.facets[name]}
            value={filters[name]}
            onChange={(next) => {
              set(name, next);
            }}
          />
        ))}
      </div>
      <FilterBar
        filters={applied(filters).map(({ name, value }) => ({
          name: facetLabel(name),
          value:
            name === "q" ||
            name === "part" ||
            name === "kind" ||
            name === "tool" ||
            name === "owner"
              ? value
              : wordFor(name, value),
        }))}
        onRemove={(label) => {
          const found = searchNames.find((n) => facetLabel(n) === label);
          if (found !== undefined) set(found, undefined);
        }}
        onClear={() => {
          onFilters({});
        }}
      />
      <p data-part="rules-count" role="status">
        {say("rules.shown", {
          shown: String(counts.shown),
          total: String(counts.total),
        })}
      </p>
      {groups.length === 0 ? (
        <EmptyFrame title={say("rules.empty")} />
      ) : (
        groups.map((group) => (
          <section
            key={group.slug}
            data-part="rules-group"
            data-group={group.slug}
          >
            <ListTable
              caption={say("rules.table", { part: group.name })}
              columns={[
                { key: "rule", label: say("rules.col.rule") },
                { key: "statement", label: say("rules.col.statement") },
                { key: "strength", label: say("rules.col.strength") },
                { key: "standing", label: say("rules.col.standing") },
                { key: "owner", label: say("rules.col.owner") },
                { key: "layer", label: say("rules.col.layer") },
                { key: "holds", label: say("rules.col.holds") },
              ]}
              rows={group.rules.map((rule) => rowOf(rule, bundle, ruleHref))}
            />
          </section>
        ))
      )}
    </>
  );
}

export function RulesView({
  region,
  filters,
  onFilters,
  onRetry,
  ruleHref = ruleAddress,
}: RulesViewProps): ReactElement {
  return (
    <div data-part="rules-view" data-state={region.state}>
      <h1>{say("shell.view.rules")}</h1>
      <p className="gr-rules-question">{say("rules.question")}</p>
      {body(region, filters, onFilters, onRetry, ruleHref)}
    </div>
  );
}

function body(
  region: DataState<Bundle>,
  filters: Filters,
  onFilters: (next: Filters) => void,
  onRetry: () => void,
  ruleHref: (id: string) => string,
): ReactElement {
  switch (region.state) {
    case "loading":
      return (
        <p role="status" data-part="rules-loading">
          {say("rules.loading")}
        </p>
      );
    case "empty":
      return region.why === "missing" ? (
        <ErrorFrame message={missingFrame(region.file)} onRetry={onRetry} />
      ) : (
        <EmptyFrame title={noRowsFrame(region.file)} />
      );
    case "error":
      return <ErrorFrame message={failure(region.cause)} onRetry={onRetry} />;
    case "disconnected":
      return (
        <ErrorFrame message={say("rules.disconnected")} onRetry={onRetry} />
      );
    case "stale":
      return (
        <>
          <Banner tone="warning">{staleBanner(region.ageMs)}</Banner>
          <Loaded
            bundle={region.value}
            filters={filters}
            onFilters={onFilters}
            ruleHref={ruleHref}
          />
        </>
      );
    case "ready":
      return (
        <Loaded
          bundle={region.value}
          filters={filters}
          onFilters={onFilters}
          ruleHref={ruleHref}
        />
      );
  }
}
