---
type: reason
status: current
as-of: 2026-11-16
---

# Why the Postgres rules are shaped as they are

Seven rules of the postgres part (engine, migrations, queries and schema) rest on the reasons below. Each rule keeps its status.

- **postgres.engine.1** (Hard, active): An engine version is part of the contract, and a bug fixed in a later version is a fault carried until the move. A neighbouring engine's write-ahead-log bug stood for years, and a company using it saw corruption in its backup snapshots. Agents bump an image tag to make a build pass (inference).
- **postgres.migrations.1** (Hard, active): A gap or a reorder changes what replay produces, a schema file that drifts from replay misleads the types and the review, and an edited landed migration means two databases that ran different text under one number.
- **postgres.migrations.5** (Hard, active): A down script gives false comfort for a step that cannot be reversed, and an unstated delete behaviour is a decision nobody made. A migration that fails halfway at start leaves a store no version can read unless the copy from just before is put back.
- **postgres.queries.1** (Hard, active): A query written as a string fails at run time on a renamed column; types from the model fail the build instead. One model that generates every other shape is the strongest guard against a type, a table and a contract drifting apart over years, and a source chosen once cannot be argued about per change.
- **postgres.schema.1** (Hard, active): A convention held only in migration text drifts from what the database actually holds. A kind-and-id text reference is one the database cannot check, and a law tested only through a copy in code proves the copy.
- **postgres.schema.5** (Hard, active): A recorded fact that can be changed is not a record, and a trigger outlasts every application that writes to the table.
- **postgres.schema.6** (Hard, active): A library schema lets a library be upgraded without touching the product's tables, and keeps the model-equals-DDL check (postgres.migrations.1) from failing on tables the model does not own.

## Rules it serves

arch.decisions.1, arch.decisions.4, docs.links.2
