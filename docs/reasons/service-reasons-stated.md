---
type: reason
status: current
as-of: 2026-11-16
---

# Service rules that carry their reason in this page

Fourteen rules of the service part carry their reason in this one page. Each states the strength and status it has in the rulebook and the measurement or incident behind it.

- **service.authz.2** (Hard, active): A catalogue kept by hand drifts from the routes, and access reviews then read a list that is wrong.
- **service.authz.5** (Hard, active): Field filtering written per route is forgotten per route; a field leaks from the route that missed it.
- **service.authz.6** (Hard, active): A mapping to a retired permission grants nothing or, worse, something else.
- **service.authz.9** (Hard, active): Every service that talks to the identity provider can change its users and credentials.
- **service.contract.6** (Hard, active): A caller written from the contract cannot handle an outcome it was never told about.
- **service.contract.9** (Hard, active): Caches, prefetchers and retries call GET freely; a GET with effects turns each of them into a write.
- **service.cycle.3** (Lens, active): The integration tiers of eleven services in one set of projects, 68 and 229 seconds in two of them, ran outside every hook and nothing proved them; two packages' own gates skipped 24 and 15 database tests quietly.
- **service.cycle.4** (Lens, active): One real order was lost in one set of projects with every unit suite green, because nothing read the state after the response.
- **service.cycle.5** (Lens, active): Six packages of one set of projects publish with no workflow, and one service has no deploy on record.
- **service.errors.3** (Hard, active): A raw message leaks internals and changes with every refactor.
- **service.events.1** (Hard, active): Two definitions of one event drift, and the break shows up in a consumer in production.
- **service.jobs.1** (Hard, active): Each service inventing its own run log leaves no single answer to what ran last night and what failed.
- **service.outbound.2** (Hard, active): A call without a timeout holds a worker for as long as the callee stays silent, and the failure spreads upstream.
- **service.outbound.6** (Hard, active): A client that turns a failure into an empty result passes the fault downstream, where it looks like missing data.

## Rules it serves

arch.decisions.1, arch.decisions.4, docs.links.2
