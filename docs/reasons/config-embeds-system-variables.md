---
type: reason
status: current
as-of: 2026-11-22
---

# The config package embeds its one table of system variables

`internal/config` embeds `internal/config/vars_system.txt`, the rows of the settings help table that name a system's variables, so that no Go file of gragas names a system. `gate/subsets.json` lets that package past `no_embeds` in the subset build, as it lets `internal/cli/planned`, and the row names the file: an allowed package may carry a `files` list, and the check is red unless the package's embedded files are exactly that list. A second embedded file in `internal/config` is red; the table's one file is the green control.

The schema, `gate/schema/subsets.schema.json`, takes the optional `files` list on an allowed entry. `no_embeds` stays on.

## Why

[The embedded data folders rule](embedded-data-folders.md) keeps the explorer's files out of the engine's subset build, and `no_embeds` is how the build proves it. The table of variables is a data file of the engine, not the explorer's, and could not stay a Go literal without naming the systems. A package allowed by name alone would let any later embedded file travel in with it; naming the file makes the allowance as narrow as the need.

## What was weighed instead

- Keeping the rows as a Go literal. It puts a system's name in Go code, which the product refuses.
- Allowing the package without naming the file. A second embedded file, the explorer's among them, would pass unseen.
- Turning `no_embeds` off for the subset. It is the whole of the proof that the explorer stays out.

## Rules it serves

arch.layers.13, arch.shared.7
