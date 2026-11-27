---
type: reason
status: current
as-of: 2026-11-16
---

# Why the Go rules are shaped as they are

Eight rules of the Go part (concurrency, errors, layout, logging and style) rest on the reasons below. Each rule keeps its status. There is no push CI: the gate runs locally, and a check too slow for the hook moves to the release workflow, not to a hosted job.

- **go.concurrency.6** (Hard, active): Practice. No repository uses the leak library, and only one runs the race detector by default. The profile found one planted leak in verification. A check too slow for the hook moves to the release workflow.
- **go.concurrency.7** (Hard, active): Concurrency is the largest source of bugs that show only years in, and a measured reason at the call is the cheapest guard. Keeping adapters free of polls is what keeps a program idle when nobody asks.
- **go.errors.4** (Hard, active): Measured: 0 exits outside the main package everywhere today; a library that exits cannot be tested or reused. The part that protects is the lint: no exit outside main, so every path is testable in process. Both Go programs this rulebook targets return an ending because a refusal is a value they print on three surfaces, and forcing an error type in between adds a layer with no reader.
- **go.errors.8** (Hard, active): Typing an error again after a flattening joint keeps callers off message strings that change; it is the Go side of the one refusal value of cli.errors.2.
- **go.layout.1** (Hard, active): Practice; all six of the project's Go repositories follow cmd and internal. A shared layout makes a port, an adapter or a command findable across programs by an agent that has read one of them, and the two Go target programs already agree on the names.
- **go.logging.1** (Hard, active): Measured: only one repository uses slog (3 files); another has a log.Fatal. One logging API with a choice of handler keeps styled terminal output and machine logs from being two code paths.
- **go.style.5** (Hard, active): A helper without t.Helper reports the failure at the helper's line, not the caller's, and a case named by its claim is the failure message a person reads first. The standard package keeps tests readable by anyone who knows Go.
- **go.style.6** (Hard, active): A suite that passes with its subject deleted is the cheapest mutation test there is, and siblings unresolvable is what makes a package extractable in fact, not only by the import table.

## Rules it serves

arch.decisions.1, arch.decisions.4, docs.links.2
