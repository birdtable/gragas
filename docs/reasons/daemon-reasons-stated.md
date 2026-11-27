---
type: reason
status: current
as-of: 2026-11-16
---

# Why the daemon rules are justified in one page

Seven rules of the daemon part carry their reason in this one page. Each rule keeps its status, and each states below the measurement, incident or practice that justifies it.

- **daemon.access.5** (Hard, active): A client with no timeout hangs a CLI too, so this applies to any program that builds an HTTP server or client. A construction analyzer was considered and not chosen: forbidigo on the type name also hits parameter and return types (a verified false positive).
- **daemon.health.1** (Hard, active): Established practice; the shared handler is what makes the shape the same.
- **daemon.lifecycle.1** (Hard, active): Measured on the project's own programs: of eight, one has a working guard, three do it manually and four have none.
- **daemon.logging.2** (Hard, active): Backed by a test that is planned, not yet written.
- **daemon.resilience.1** (Hard, active): Established practice; unbounded work turns a burst into an outage. Part of the supervision row, whose restart half is daemon.units.3.
- **daemon.resilience.4** (Hard, active): Practice only; it would become a rule with a duplicate-effect fault found in one of the daemons. The command-side twin is cli.commands.2.
- **daemon.units.1** (Hard, active): Established practice; a malformed plist is silently ignored by the manager.

## Rules it serves

arch.decisions.1, arch.decisions.4, docs.links.2
