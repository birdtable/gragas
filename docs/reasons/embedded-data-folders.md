---
type: reason
status: current
as-of: 2026-11-08
---

# Where the engine's embedded data lives and how the subset build is made

The engine's embedded data folders live in `internal/host/assets/`: the handbook, the profiles, the fixture rulebook, the contract and format schemas, and the explorer's placeholder page and marked copy. The one `go:embed` site is the `host/assets` package. A rulebook's and a project's own `adapters/` and `profiles/` override folders keep their names; only the engine's embedded copies are in `host/assets`. A tool is a plugin that is installed, not embedded, so the embedded folders hold no tool adapters.

The explorer's subset build is `cmd/gragas-subset`, with no build tag. It is the shipped command tree with the Assets port filled by an adapter that answers `ErrUnavailable`, and it never imports `host/assets`. The Assets port is filled by a function argument, never by a build tag. The gate builds the subset in every run and fails if its package list holds `host/assets` or any package of the module with an embedded file. Nothing ships it. A folder with nothing to embed yet holds a `.placeholder` file that says what will fill it; the adapter hides that file.

## Why

`go:embed` reads only below its own package, and go.layout.1 keeps programs under `cmd/` and everything else under `internal/`, so the folders the binary embeds sit in the one adapter that embeds them. A build tag is not an interface, and go.layout.1's repository test refuses a build constraint outside `host/`, so the subset build is a second entry that passes a different value for the port, as an optional port is where its capability is missing.

## What was weighed instead

- A root package that embeds the folders at the repository root. It lost because go.layout.1 allows no package outside `cmd/` and `internal/`.
- A `subset` build tag in `cmd/gragas`. It lost because a build tag is not an interface, and a build constraint outside `host/` is refused.
- Copying the folders into the adapter at build time. It lost because it puts a generated or committed build product in the tree.

## Rules it serves

arch.layers.13 (a subset build without the separable part, every gate run), arch.ports.6 (go:embed in one adapter package), go.layout.1 (programs under cmd/, the rest under internal/), arch.ports.5 (a port is never a stub that pretends to work).
