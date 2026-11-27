---
type: rubric
status: current
as-of: 2026-11-08
---

# Explorer design rubric

Review judges every explorer view against this rubric (web.delivery.4). It is a lens, not a gate:
a reviewer reads a board or a rendered view with the questions below, in order, and writes down
the first one that fails. The measured parts, states and breakpoints a board must carry are in
[Board contract](#board-contract). The numbers a test can measure are held by tests, not by this
page (web.parts.17).

## The hierarchy test

Read this first. Look at the view for three seconds, then close it and answer:

1. What is this view for? The answer is the one thing the eye lands on first.
2. What is the standing of what I am looking at, and what can I do next? These are second.
3. Everything else is third and may be quiet.

A view fails the hierarchy test when two things compete for first, when standing is not in the
first two, or when the first thing is chrome (a banner, a filter bar, a badge). Fix the
hierarchy before any other finding: size, weight and position are decided by it, and the other
questions are judged on the fixed view.

## Tone

- Words are plain and short, and every user-visible word comes from the message module
  (web.content.1).
- A state says what is true and what to do. "No rules" with the handbook link, not "Oops".
- No joke, no aside, no exclamation mark.
- A number is a number with its unit, never "a few" or "many".

## Brand fit

- Colour, size and font come from the token document and nothing else (web.tokens.8).
- The look is the site design canvas. A board that departs from it says why, on the issue,
  before it changes.
- Light and dark are both drawn, and both are read.

## Standing

Standing is never carried by colour alone: the word and the symbol are on every row, in both
themes (web.accessibility.4).

## Board contract

Every board for a view carries these. A board that lacks one is not reviewed.

| Item | Value |
|---|---|
| Widths | three named breakpoints: phone 390, tablet 900, desktop 1440 |
| States | Default, Loading, Empty, Error, Stale; the here view also draws no report |
| Themes | light and dark |
| Pointer targets | at least 24 by 24 CSS px, and 44 on touch (web.layout.5) |
| Horizontal scroll | none at 390 or 320, in any state and both themes (web.layout.1) |
| Text growth | the board shows the view with text grown, without loss (web.accessibility.6) |
| Parts | each named region has a `data-part` name and, where the design fixes one, a measured width, height or count (web.parts.17) |
| Names | made-up names only, from the stand-in list (tests.doubles.7) |

## What review writes down

One line per failed question: the question, the view and width, and the part by its name. A
finding that points at "the top" or "it feels heavy" is sent back; point at a `data-part`.
