---
type: reason
status: current
as-of: 2026-11-16
---

# Postgres rules carry their reason in this page

Two Postgres rules cite this page. Each states its own reason below, with the strength and status it has in the rulebook.

- **postgres.client.4** (Hard, active): A stronger isolation level obliges the application to retry, and a helper without the retry loses the write or fails the user. A forced serialisation failure on a real container is the only ordinary test that sees it.
- **postgres.migrations.6** (Hard, active): A migration on nearly empty tables cost one payment company about 15 seconds of API downtime: the ALTER waited for a strong lock behind a long read and every later query queued behind it.

## Rules it serves

arch.decisions.1, arch.decisions.4, docs.links.2
