---
type: reason
status: current
as-of: 2026-11-16
---

# What a stored reviewer verdict does

A stored reviewer verdict is evidence for a human or a gate to weigh, not a verdict that passes a change on its own. Twenty-five rules cover review, build, typing, linting, routing and runtime settings, and each states its own reason, with the strength and status it has in the rulebook.

- **agents.review.1** (Hard, active): Argued: no study supports AI review as a verdict. The project's own evidence is that review finds faults: one project's designer agent found 31 of 72 and another's reviewers 16, and a spec-against-diff lens found a missing case that tests had not.
- **astro.build.1** (Hard, active): On-demand rendering adds a server and an adapter to a site that was built to be a folder of files. The rule rests on the framework's own guides; no site uses either feature yet.
- **react.components.1** (Hard, active): A leaf part that fetches cannot be reused or tested without the network, and data access spreads into the design system.
- **react.lint.1** (Hard, active): A missing dependency or a conditional hook is a bug that shows only at run time. Tried on eight planted files (three incompatible-library warnings, two query errors, four clean cases); never run on a real tree.
- **react.routing.1** (Hard, active): A hand edit to generated code is lost at the next generation, and a stale tree routes to pages that no longer exist. The banner and a build with no inline script were tried on one router plugin version; the check has never run on a real React tree.
- **react.routing.2** (Hard, active): A typo in a path compiles in an untyped router and fails only when someone follows the link. In a trial the typed router rejected five of five wrong usages and a second router package rejected none.
- **react.routing.3** (Hard, active): The URL is input from outside; an unvalidated search value reaches a query as the wrong type. This is the React router form of the rule that a page's place is carried in the URL.
- **react.routing.4** (Hard, active): Settings spread over route files drift apart, and a browser global at module scope breaks any non-browser render of the tree, which includes tests and static builds.
- **react.routing.5** (Hard, active): Two caches show two answers for one resource, and a restore that finishes after mount flashes empty or stale data and can revive a gated read.
- **react.server-state.1** (Hard, active): Two stores for one resource show two answers, and a copy in local state goes stale without a signal. The import ban and the lint hold the mechanical half; the gated-read half is held by a sweep.
- **react.testing.1** (Hard, active): A private router in a test hides provider and routing bugs that the real app has.
- **ts.compiler.1** (Hard, active): A type error that the gate lets through is a fault found at run time. In one reference project the type check refused a wrong ruling by the project's own lead on one change, which is the evidence for a hard type gate.
- **ts.compiler.2** (Hard, active): Turning on all flags in one change produces a wall of errors that gets suppressed; one flag per change keeps each cost small and visible.
- **ts.compiler.3** (Hard, active): One base changes once and reaches every project by a version bump, where per-project files drift. A library and an app are different compile targets and no single module mode serves both, while the strict flags are what the rule exists to share.
- **ts.compiler.4** (Hard, active): A flag that is off with no recorded reason is indistinguishable from one forgotten, and nobody turns it on.
- **ts.errors.1** (Hard, active): The reader-facing rule covers what the person sees, and the runtime listener catches errors at run time; nothing covered the source.
- **ts.lint-config.4** (Hard, active): A cold lint is too slow for an edit loop, so it gets skipped.
- **ts.lint-config.9** (Hard, active): A plugin whose peer range excludes the project's TypeScript installs with a warning and then fails or silently skips rules.
- **ts.lint.1** (Hard, active): A promise that is neither awaited nor handled fails silently and out of order. Typed ESLint costs 3.3 to 4.4 times the untyped run (a reference measurement); oxlint took 0.6 CPU seconds for the same 3,633 files.
- **ts.lint.4** (Hard, active): A loose schema parses everything and checks nothing, which defeats the parse-at-the-edge rule.
- **ts.runtime.1** (Hard, active): Two Node majors in one project mean the code that passes tests runs on a different runtime in production. Neither of two reference sites declared Node at all.
- **ts.type-safety.1** (Hard, active): One `any` turns off checking for everything it touches, and spreads through inference. In a reference repository 118 non-null assertions sat in one baseline; `any` and the empty record are the worst holes at a signature.
- **ts.type-safety.2** (Hard, active): An assertion is a claim the compiler cannot check; one wrong claim removes the safety of a whole path.
- **ts.type-safety.4** (Hard, active): A cast response is trusted without a check; a server change then breaks callers far from the call.
- **ts.type-safety.8** (Hard, active): An untyped dependency brings `any` in through its imports.

## Rules it serves

arch.decisions.1, arch.decisions.4, docs.links.2
