---
type: reference
status: current
as-of: 2026-11-09
---

# Proposal template: changing the format, the contract or an adapter's shape

A change to the rulebook format, to the contract or to an adapter's shape arrives as a proposal to this repository under this template (S25). It is decided in public, and the options it rejects are recorded with the reason for each.

Copy the headings below into a pull request or an issue titled with what changes, and fill each in. A proposal missing a heading is returned.

## Motivation

What cannot be said or done now, and who is stopped by it. Name a case from a real rulebook or from the fixture, not a general wish.

## Change

The change itself, as the text of the spec it would make true. Name every schema file and every field it adds, renames or removes, with the required-field rule of each, and say which page of [the format spec](../format/README.md) gains or loses a row.

## Compatibility

Who reads the new shape and who still writes the old. Say whether the change adds a field with a default (a minor) or removes or reshapes one (a major). A removed field keeps a window of two minors in which both shapes are accepted. Say what a reader on the previous major version does when it meets the new shape.

## Migration

What `migrate` does to a rulebook written the old way, as a plan the owner answers yes to, and what it cannot do alone. Say how a rulebook is checked after the move.

## Options that lost

Each option considered and not chosen, with the reason it lost. The decision log keeps them so the same option is not proposed again.

## Decision

Filled in by the maintainer who decides: the decision's id in the log, the date, and who decided.
