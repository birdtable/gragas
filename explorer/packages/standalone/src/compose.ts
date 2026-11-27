// The one file that imports every view's manifest (arch.ports.13):
// a view ticket replaces its row with `defineView(<its manifest>)`, imported
// from core's views, and touches no other file of the shell. A row still
// holding the shell's stand-in is a view that has not landed, and the test
// holds the rows that do against the folders of core/views.
import { rulesManifest } from "@gragas/core/views";
import {
  AttentionPage,
  attentionSearchSchema,
} from "@gragas/core/views/attention";
import {
  ChangelogPage,
  changelogSearchSchema,
} from "@gragas/core/views/changelog";
import { CoveragePage, coverageSearch } from "@gragas/core/views/coverage";
import { HealthPage } from "@gragas/core/views/health";
import {
  RelationsPage,
  relationsSearchSchema,
} from "@gragas/core/views/relations";
import {
  RulebooksPage,
  rulebooksSearchSchema,
} from "@gragas/core/views/rulebooks";
import { RulePage, ruleSearchSchema } from "@gragas/core/views/rule";
import { TermsPage, termsSearchSchema } from "@gragas/core/views/terms";
import { standingManifest } from "@gragas/core/views/standing";
import { toolsManifest } from "@gragas/core/views/tools";
import { defineView, noSearchSchema } from "./manifest.ts";
import { viewIds, type ViewId } from "./views.ts";

export const manifests = {
  rules: defineView(rulesManifest),
  rule: defineView({ search: ruleSearchSchema, Page: RulePage }),
  relations: defineView({
    search: relationsSearchSchema,
    Page: RelationsPage,
  }),
  rulebooks: defineView({
    search: rulebooksSearchSchema,
    Page: RulebooksPage,
  }),
  tools: defineView(toolsManifest),
  coverage: defineView({ search: coverageSearch, Page: CoveragePage }),
  standing: defineView(standingManifest),
  health: defineView({ search: noSearchSchema, Page: HealthPage }),
  changelog: defineView({
    search: changelogSearchSchema,
    Page: ChangelogPage,
  }),
  terms: defineView({ search: termsSearchSchema, Page: TermsPage }),
  attention: defineView({ search: attentionSearchSchema, Page: AttentionPage }),
} as const satisfies Readonly<Record<ViewId, unknown>>;

/** The views whose pages have landed. */
export const registered: readonly ViewId[] = viewIds.filter(
  (id) => manifests[id].placeholder !== true,
);
