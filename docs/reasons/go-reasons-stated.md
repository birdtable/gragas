---
type: reason
status: current
as-of: 2026-11-16
---

# Why the Go rules are justified in one page

Eight rules of the Go part carry their reason in this one page. Each rule keeps its status, and each states below the measurement or practice that justifies it.

- **go.concurrency.3** (Hard, active): Own bugs: 5 of 60 concurrency bugs, half of the row. 0 real hits today in six repositories, so adoption is free.
- **go.concurrency.4** (Hard, active): Measured: used in 5 of 6 repositories. A daemon that ignores the interrupt is killed by the service manager and loses its state.
- **go.concurrency.5** (Lens, active): Own bugs: 5 of 60 sampled bugs were state that outlived its cause. Cause and end are semantic: no tool knows a state has a cause.
- **go.cycle.4** (Lens, active): A second machine's first push was red on a module graph that only an earlier run had fetched.
- **go.errors.2** (Hard, active): Practice; a wrapped error compared with == silently stops matching.
- **go.errors.7** (Hard, active): Practice, with no fault in the project's record. Run on two repositories: 0 real hits, so adoption debt is zero where tried.
- **go.layout.4** (Hard, active): A cgo binary links the host's C library and breaks the static, copy-anywhere build. Measured: set in 5 of the 7 programs today (verified); the library repository and one scraper set none.
- **go.style.1** (Hard, active): Practice. One repository declares 98 interfaces; their sizes were not measured; the linter flagged a 40-method port in another.

## Rules it serves

arch.decisions.1, arch.decisions.4, docs.links.2
