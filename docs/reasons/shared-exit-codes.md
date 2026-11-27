---
type: reason
status: current
as-of: 2026-11-16
---

# Programs share one set of exit codes

Every command-line program uses the same exit codes, and the shared module re-exports them so there is one place they can change. The refused code is 5. The rules below follow from that. Each states its own reason, with the strength and status it has in the rulebook.

- **cli.environment.8** (Hard, active): The refused code is 5. One shared --yes is what lets an agent run any command non-interactively without learning a flag per prompt.
- **cli.exit.1** (Hard, active): Measured today: one program uses 0, 1, 2, 64, 69, 75 and 78; the shared module uses 0 to 4 with 1 for both refused and failed; three others use 1 for everything; one TypeScript program uses three codes with refusal and bad usage both 1.
- **cli.exit.2** (Hard, active): The shared module re-exports the codes so there is one place they can change.
- **docs.glossary.3** (Hard, active): Two programs that each spell out a code or status word drift apart, and a code that is also a sentence cannot be changed without breaking readers.

## Rules it serves

arch.decisions.1, arch.decisions.4, docs.links.2
