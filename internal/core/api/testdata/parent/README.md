# The parent's bundle, a stand-in

`bundle.json` is the bundle of `fixture-parent` 0.9.0, the parent the base
fixture (`internal/host/assets/fixture/base`) pins in its `extends`. It stands
in until a bundle writer makes a parent's bundle from a release: it is
written by hand, and a bundle has no field a comment could go in, so this file
says it.

Tests seed it into the releases cache, at the path `coreapi.ParentBundle` names
for the pin, and `rule exclude` and `rule include` read it through
`contract.ReadBundle` (A50). It is held to the contract: the journeys read it
through the program's own schemas, and `internal/host/jsonschema` checks it
against the bundle schema.

| Rule | Status | Binding | Used by |
|---|---|---|---|
| `std.errors.1` | active | golangci-lint source wrapcheck, warning, rung 1, commit | tightened to error by the base fixture's `bindings/std-errors-1.yaml`; bind and rebind in the journeys |
| `std.errors.2` | active | | excluded by the base fixture |
| `std.errors.3` | active | | excluded, then lifted, by the base fixture |
| `std.errors.4` | active | golangci-lint source errorlint, error, rung 1, commit | exclude, then include, in the journeys; a weakening rebind is refused |
| `std.errors.5` | retired | | include ends not ready |

The bindings are what an overlay's own binding of a parent's rule is held to
(S34, S70): the overlay may tighten them by a record of its `bindings/`, and a
change that weakens one, moves it, or changes its rung or run point is
refused.
