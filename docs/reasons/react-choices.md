---
type: reason
status: current
as-of: 2026-11-16
---

# The React part's target-state choices

The React part settles how client state, server state, components, routing, linting and tests are done. Twenty rules follow from those choices. Each states its own reason, with the strength and status it has in the rulebook.

- **react.client-state.1** (Lens, active): Two stores for client state drift apart, and state in context re-renders every consumer. Half of the rule holds by package ban; the context half is a model lens.
- **react.client-state.2** (Hard, active): With no form in either site the check is vacuous until the detector sees one. Two form libraries, or field errors set by hand in pages, give two patterns for fields, validation and server errors.
- **react.client-state.3** (Hard, active): Stored derived state drifts from the value it was derived from.
- **react.components.1** (Hard, active): A leaf part that fetches cannot be reused or tested without the network, and data access spreads into the design system.
- **react.components.2** (Hard, active): Hand-built widgets drift from the accessibility contract: in the source a hand-built combobox was 416 lines and a date picker 371 lines. The chosen library gave zero violations under the script and style policy.
- **react.components.3** (Hard, active): Markup built from data is the way untrusted input reaches the page as script.
- **react.components.4** (Hard, active): An inline style carries colours, sizes and fonts past the token theme and the colour-literal lint; a regex that matched only some shapes reported zero and missed the rest.
- **react.config.1** (Hard, active): The React twin of the template-stack rule. `VITE_` and `import.meta.env` had 0 hits in the catalogue, so the rule is held for the absence.
- **react.lint.1** (Hard, active): A missing dependency or a conditional hook is a bug that shows only at run time. Tried on eight planted files (three incompatible-library warnings, two query errors, four clean cases); never run on a real tree.
- **react.lint.2** (Hard, active): The hooks and query plugins alone do not catch a list without keys or a component defined inside a component. The rule was added when a search of the catalogue found no other rule that holds it.
- **react.lint.3** (Hard, active): A label, role or key handler missing in markup is found cheaply at lint time, before the accessibility run in a browser.
- **react.routing.11** (Hard, active): Logic in an entry or a route file escapes the module's tests and boundaries; both entries complied when counted.
- **react.routing.3** (Hard, active): The URL is input from outside; an unvalidated search value reaches a query as the wrong type. This is the React router form of the rule that a page's place is carried in the URL.
- **react.routing.4** (Hard, active): Settings spread over route files drift apart, and a browser global at module scope breaks any non-browser render of the tree, which includes tests and static builds.
- **react.routing.5** (Hard, active): Two caches show two answers for one resource, and a restore that finishes after mount flashes empty or stale data and can revive a gated read.
- **react.server-state.1** (Hard, active): Two stores for one resource show two answers, and a copy in local state goes stale without a signal. The import ban and the lint hold the mechanical half; the gated-read half is held by a sweep.
- **react.server-state.2** (Hard, active): Freshness set per call site drifts, and two key namespaces that collide invalidate each other's data.
- **react.server-state.5** (Hard, active): An effect that fetches has no cache, no dedupe, no cancel and races on fast navigation.
- **react.testing.1** (Hard, active): A private router in a test hides provider and routing bugs that the real app has.
- **react.testing.2** (Hard, active): A hand stub tests the stub; a sample from the server tests the contract.

## Rules it serves

arch.decisions.1, arch.decisions.4, docs.links.2
