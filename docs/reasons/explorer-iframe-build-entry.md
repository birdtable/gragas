---
type: reason
status: current
as-of: 2026-11-08
---

# The explorer's iframe is a second build entry, without the here view

The iframe is a second build entry of `standalone`, built without the here view. The here view lives under `standalone` in its own part, and `element` takes a `view` attribute naming any of the eleven rulebook views.

## Why

The here view belongs to the standalone packaging only, and a single build with a switch ships its code to the iframe anyway. One composition file per entry keeps arch.ports.13 true for each entry, and the gate lists the modules of every output so a leak fails. With the `view` attribute, a host mounts one view or several and never receives the here view.

## What was weighed instead

- One build with a runtime or build-time flag. The code still ships, or the flag is read at run time.
- The here view inside `core`. The element would then import it.
- A React package beside the element. It would give two mounts for the same views.

## Rules it serves

arch.ports.13, arch.ports.8, ts.modules.4
