---
type: reason
status: current
as-of: 2026-11-16
---

# Reasons for twenty-three rules of the cli part

Twenty-three rules of the cli part carry their reason in this one page. Each keeps its status and cites this record. Each reason is a measurement or an incident, stated without naming the project it came from.

- **cli.commands.10** (Hard, active): The command framework supplies suggestions and the test proves they are on. One program prints its version line on stdout after a typo.
- **cli.commands.16** (Record, active): No tool decides that an experience is good.
- **cli.commands.2** (Hard, active): Shares the fixtures and helper of cli.commands.1, so no extra build. Not measured in Go.
- **cli.commands.4** (Hard, active): 2 of 60 sampled bugs were a write beyond the command's scope.
- **cli.commands.5** (Hard, active): One repository has an install-is-a-rename test; the shared test is proposed.
- **cli.commands.6** (Hard, active): A count that only falls would punish legitimate idempotent deletes, so there is no number.
- **cli.cycle.1** (Lens, active): One project kept goldens at 80, 60 and 40 columns; each output change meant three diffs nobody read, and the 60 and 40 goldens went stale on the main branch unnoticed.
- **cli.cycle.2** (Lens, active): One project's pty control missed its golden under load, at load 76 to 202 on one platform and only under load; the fix was to read until the frame, not longer.
- **cli.environment.1** (Hard, active): All four CLIs print escape codes with both `NO_COLOR` and the force variable set (292, 150, 216 and 256 escapes); the shared module passes and no repository gates on it. This is the fault that earns the matrix helper.
- **cli.environment.10** (Hard, active): The test is planned.
- **cli.environment.11** (Hard, active): A status that says ok only by a green colour is invisible without colour.
- **cli.environment.2** (Hard, active): The matrix is trialled and a copy printing 120 to 179 cells at `COLUMNS=200` fails.
- **cli.environment.3** (Hard, active): The silent-pty trial fails everywhere today, the shared module included; the fix comes first, and adoption is held to that order.
- **cli.environment.5** (Hard, active): The matrix is trialled and passes in all programs today.
- **cli.environment.7** (Hard, active): The matrix is planned; the shared module already has the switch.
- **cli.environment.9** (Hard, active): The test fails today: a live-update loop still ticks after focus-out.
- **cli.errors.3** (Hard, active): Forbidigo matches identifiers, not argument text; its licence shows as unasserted on GitHub, and golangci-lint bundles it.
- **cli.errors.4** (Hard, active): A sanitiser in the shared module with a test; the structured log handler covers logs; a raw print is cli.errors.3's to catch.
- **cli.look.2** (Hard, active): The contrast test is held in the module and all four copies but covers 4 of 9 colours, so a colour can be added with no contrast entry.
- **cli.look.3** (Hard, active): Hand-typed count and size formats drift; the scan is planned.
- **cli.look.5** (Hard, active): Constructors only make the closed set a compile fact.
- **cli.look.6** (Hard, active): Hand-typed footers drift; related to the command rule that a dry run ends by naming the apply command (cli.commands.1).
- **cli.look.7** (Hard, active): The current table drops and cuts a column at 40 columns.

## Rules it serves

arch.decisions.1, arch.decisions.4, docs.links.2
