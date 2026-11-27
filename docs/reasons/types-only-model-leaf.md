---
type: reason
status: current
as-of: 2026-11-08
---

# A types-only model leaf and independent contexts

The model is a leaf that holds types only. Contexts never import or call each other, and the list of contexts is data in `layers.yaml`.

## Why

Arrows between contexts that run in both directions cannot be imports. Shared types placed in a sibling context make two packages one. A contract the lint cannot read is a wish, so the contexts are listed as data the lint reads.

## What was weighed instead

- Interfaces between contexts for every arrow. This gives many small interfaces that all name the same model.
- A contract stated only in prose. The lint cannot check it.

## Rules it serves

arch.ports.8, arch.layers.7, arch.layers.9, arch.layers.11
