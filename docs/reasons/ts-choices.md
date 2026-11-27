---
type: reason
status: current
as-of: 2026-11-16
---

# TypeScript rules and the choices behind them

The TypeScript part of the rulebook covers the compiler base, lint configuration, module style, error handling, type safety and the schema that every other shape is generated from. This page gives the reason for each rule.

## The rules

- **ts.compiler.3** (Hard, active): One base changes once and reaches every project by a version bump, where per-project files drift. A library and an app are different compile targets and no single module mode serves both, while the strict flags are what the rule exists to share.
- **ts.compiler.5** (Hard, active): A shared rule that a local file can silently override is not shared. Capping the overlay to three keys is the maintenance win: nobody loosens strict in a corner.
- **ts.compiler.6** (Hard, later): Running both compilers and failing on a disagreement is the safety; a window that depends on a scheduled tier cannot be met when gates run only at the push and the release. Until the native release is stable nothing can fail, so the rule waits.
- **ts.errors.1** (Hard, active): The reader-facing rule covers what the person sees, and the runtime listener catches errors at run time; nothing covered the source.
- **ts.errors.2** (Hard, active): A rethrow without a cause loses the stack that says where it started.
- **ts.errors.3** (Hard, active): It is noise that reads like handling.
- **ts.errors.4** (Hard, active): Console output in a page is invisible to the log pipeline and leaks internals to the browser.
- **ts.gates.1** (Hard, active): A gate that exits early skips the rules after it and can cut its own output, and a gate with no handler can die on a throw and leave a zero exit code; both report green for work that did not happen.
- **ts.lint-config.1** (Hard, active): In a trial a suppressed file grew by 250 lines and ESLint exited 0.
- **ts.lint-config.2** (Hard, active): Without pruning the list keeps room for the old violations to come back.
- **ts.lint-config.3** (Hard, active): A typed rule's result in one file depends on its imports, so a cached result is stale when another file changes. A global check is never affected-only unless sound under caching.
- **ts.lint-config.4** (Hard, active): A cold lint is too slow for an edit loop, so the agent skips it.
- **ts.lint-config.5** (Hard, active): Typed lint on every test file is the slowest part of the run, and tests use assertions legitimately.
- **ts.lint-config.6** (Hard, active): Without the override a script either fails with false errors or is left out of the lint.
- **ts.lint-config.7** (Hard, active): A parse error on a config file stops the whole lint run, or the file is silently skipped.
- **ts.lint-config.8** (Hard, active): Linting generated output produces errors nobody can fix.
- **ts.lint-config.9** (Hard, active): A plugin whose peer range excludes the project's TypeScript installs with a warning and then fails or silently skips rules.
- **ts.lint.1** (Hard, active): A promise that is neither awaited nor handled fails silently and out of order. Typed ESLint costs 3.3 to 4.4 times the untyped run; oxlint took 0.6 CPU seconds for the same 3,633 files.
- **ts.lint.2** (Hard, active): A type creates no run-time dependency, so a layer rule that forbids it forbids nothing real and invites workarounds.
- **ts.lint.3** (Hard, active): A warning is read by nobody; the gate must fail on it. The rules that matter (every rule at error, a plant per rule, type-aware checks) are engine-free, so tying the part to one engine would make the next engine change a rewrite.
- **ts.lint.4** (Hard, active): A loose schema parses everything and checks nothing, which defeats the parse-at-the-edge rule.
- **ts.modules.1** (Hard, active): A default export takes whatever name each importer gives it, so a rename does not travel, search misses uses and two files can import the same thing under two names.
- **ts.modules.2** (Hard, active): An enum and a namespace compile to runtime objects with their own semantics, so they cannot be stripped by a type-erasing loader and differ from every other type in the program; a literal union or a schema enum says the same thing and erases cleanly.
- **ts.modules.3** (Hard, active): An import without an extension resolves only under a loader that guesses, so the same module loads under the bundler and fails under the runtime, the test runner or the next loader.
- **ts.modules.4** (Hard, active): A side effect at import runs before the caller can configure, fake or refuse it, so a test, a worker or a cold start pays for work it did not ask for, and the bug shows only when the import order changes.
- **ts.modules.5** (Hard, active): A test that imports across modules passes for a module nobody could extract, so the boundary the import table claims is never exercised by the suite that should prove it.
- **ts.packages.1** (Hard, active): A root barrel is how a package grows a surface nobody chose, and only the packed tarball shows what a consumer sees: a file left out of the package or a types path that points into the source passes every check run on the tree.
- **ts.runtime.2** (Hard, active): A deprecated core module is removed in a later Node major and breaks the upgrade.
- **ts.runtime.3** (Hard, active): Two runners double the setup and split the test selection. In a reference project, 87 of the 103 test files used the built-in runner, and those are the files that must migrate.
- **ts.schema.1** (Hard, active): One model that generates every other shape is the strongest guard against a type, a table and a contract drifting apart over years, and regenerate-and-diff makes it mechanical. Naming the model as the source, and checking the live schema against it, leaves no second place where the truth can start.
- **ts.type-safety.1** (Hard, active): One `any` turns off checking for everything it touches, and spreads through inference. One reference baseline held 118 non-null assertions; `any` and the empty record are the worst holes at a signature.
- **ts.type-safety.10** (Hard, active): A declared return type is the contract the next reader builds on, and it stops an inferred type drifting under a refactor in a file the caller never sees.
- **ts.type-safety.11** (Hard, active): A clock port is what makes every time-dependent test deterministic; a direct read of the clock is a test that passes today and fails in another timezone or at midnight.
- **ts.type-safety.2** (Hard, active): An assertion is a claim the compiler cannot check; one wrong claim removes the safety of a whole path.
- **ts.type-safety.3** (Hard, active): A suppression with no reason is a bug that has been hidden. The same rule family covers no `any` and no unchecked cast without a reason.
- **ts.type-safety.4** (Hard, active): A cast response is trusted without a check; a server change then breaks callers far from the call.
- **ts.type-safety.5** (Hard, active): Supported by argument, with no fault on record. Parsed types at the boundary are what make a later adapter cheap.
- **ts.type-safety.9** (Hard, active): The share is the measure; without it the bans above are met by moving the hole, not closing it.

## Rules it serves

arch.decisions.1, arch.decisions.4, docs.links.2
