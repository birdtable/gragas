---
type: reason
status: current
as-of: 2026-11-16
---

# Why the liquid rules are justified in one page

Five rules of the liquid part carry their reason in this one page. Each rule keeps its status.

- **liquid.cycle.1** (Lens, active): A broken JSON file or schema block reaches the live theme if nothing parses it first.
- **liquid.cycle.2** (Lens, active): Eleven locale files of one theme miss 3 to 9 of 315 keys today; a check with no baseline would stay red and be ignored.
- **liquid.cycle.3** (Lens, active): A missing snippet and an unclosed conditional were both found by a prototype of this check in one theme, and nothing else would have found them before the page rendered.
- **liquid.cycle.4** (Lens, active): A theme with no linter in its gate ships the deprecations and the misused filters the platform's own tool would have named.
- **liquid.cycle.6** (Lens, active): A theme deployed straight to the live store is live before anyone has seen a page of it.

## Rules it serves

arch.decisions.1, arch.decisions.4, docs.links.2
