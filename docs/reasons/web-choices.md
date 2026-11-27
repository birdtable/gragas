---
type: reason
status: current
as-of: 2026-11-16
---

# Web rules and the choices behind them

The web part of the rulebook covers named parts, states, structure, testing and tokens. This page gives the reason for each of these rules.

## The rules

- **web.parts.17** (Hard, active): A card five pixels off its design is invisible to a unit test, and a visual test needs named parts to measure instead of a picture to compare (web.testing.5).
- **web.states.11** (Hard, active): One site tracks the link state and never draws it, and its end-to-end spec accepts either value, so it cannot fail. A page that draws 0, off or a carried-forward value for an unread one turns the server's honest answer into a false one.
- **web.structure.4** (Lens, active): A page that holds its own rules cannot be tested without rendering and drifts from the other pages. One implementation of a rule is the only one that stays right, and the server is where the test runs on real data; a page that recomputes a total is a second implementation.
- **web.testing.15** (Hard, active): A shim passes layout, focus, events and styles that a browser would fail, so a green test says little about the page people see.
- **web.testing.5** (Hard, active): A pixel threshold is either so tight it fails on a font update or so loose it passes a broken layout; a number on a named part says what is wrong and where.
- **web.tokens.15** (Hard, active): No project used it at first. It was proposed to carry the earlier lint requirement and widened to cover both frameworks. The targets style two ways over the same tokens, so the mechanism is a parameter and the tokens are what the rule holds.

## Rules it serves

arch.decisions.1, arch.decisions.4, docs.links.2
