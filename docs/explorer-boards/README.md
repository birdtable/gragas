---
type: index
status: draft
as-of: 2026-11-08
---

# Explorer boards

The boards for the explorer's twelve views at three widths: phone 390, tablet 900 and desktop 1440 (M8-02, #284). They extend the site design canvas in the companion repository and do not redraw it. The canvas itself is read at the commit the issue pins.

Review judges each board with [the rubric](rubric.md): hierarchy first, then tone, then brand fit.

## What is drawn

The design goes in two passes, so the direction is set before every board is drawn.

| Pass | Views | Status |
|---|---|---|
| 1 | relations at 390, 900 and 1440 (group A) | landed |
| 2 | rules, rule and standing bars (group B) | landed |
| 3 | rulebooks, terms and tools (group C) | drawn, in review |
| 4 | coverage, health, changelog, attention and here (group D) | drawn, in review |

Each board file in [canvas/](canvas/) is one width of one view. It holds the Default state in light and dark, then Loading, Empty, Error and Stale in both themes, then the view with text grown to 200 percent. A note sits beside every frame. The first note says who opens the view, what it conveys in order, how the layout carries that order, and the measured parts.

The parts and their numbers are in [parts.seed.yaml](../../explorer/core/parts.seed.yaml), generated from the same table the boards draw.

## Decisions for the owner after pass 1

1. **No graph at 390.** Thirteen labelled nodes do not fit in 358 px at a readable size. The phone draws the grouped list, which holds the same facts, and the graph appears from 900 up.
2. **Standing on non-rule rows.** The spec gives a standing to a rule and an observation to a binding or a tool entry. A decision, a source, a part and a term have neither, so their rows carry the kind only.
3. **The trial mark.** Spec 17.3 draws it as a hollow symbol. The boards also write the word trial beside it, so the mark never depends on telling a hollow glyph from a solid one.
4. **Standing colours.** Spec 17.2 gives proposed violet and planned amber. The canvas tokens use amber for proposed. The boards draw the six standings as the spec says, with new tokens for each.
5. **Navigation.** The header shows five of the twelve views and a More menu. The old boards showed Guides and Handbook, which are not explorer views.
6. **The missing-bundle frame.** The spec says the frame names the file it wants. The boards do not draw it yet, since the file's name is not in the spec.

## Viewing the boards

Each board is a static page and opens in a browser as it is. The site design canvas pages load a script from the design tool that is in no repository, so these boards do not load it. The pictures in [shots/](shots/) are the top of each relations board, taken with headless Chrome at the board's own width (870, 1380 and 1920, the canvas widths). They are for review and are not pixel baselines (web.testing.9).

## How the boards are made

The boards of groups B, C and D (every view but relations) are written by the scripts in [gen/](gen/). The shared module holds the style, the header, the standing chips, the states and the notes. Each view is one module. The build script writes the board files into [canvas/](canvas/), and the seed script writes the parts of each view into the seed. Run both with node from inside gen/, the build first, to write them again; the output is the same. The relations boards were written by hand from the same style and are not in the scripts. The pictures in [shots/](shots/) of the three views are the top of each board.

## What the gate holds

`internal/repo/explorer/boards_test.go` reads every board and the parts seed. It fails on a part a board names that the seed does not list, a part the seed lists twice, a data-width that is not the one the file name says, a board with no data-width, a missing frame (the five states and text grown, in light and dark), a view without all three widths, and an em dash. Each break is planted in the test too, so the checks cannot go quiet. That a rebuild from the scripts gives the same files is the `boards` member of the commit tier (`make boards`): it runs the pinned node over a copy of `gen/`, `canvas/` and the seed and compares them byte for byte. The `explorer-format` member (`make format-explorer`) runs the pinned Prettier, with its default options and only reading, over the scripts and the explorer; `.prettierignore` leaves out the generated files and any `dist/` build output (gragas#1315). The links of the pages are held by the docs test. The rendered golden and the accessibility engine are an exemption (`pages-cycle-3-boards`) until gragas#313's sweeps land.
