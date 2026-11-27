---
type: reason
status: current
as-of: 2026-11-16
---

# The cognitive complexity limit is 20

Functions are held to a cognitive complexity of 20, with tests exempt.

- **code.size.3** (Hard, active): At 20 with tests off: 94 functions in one repository, 91 in another, 48 and 33 in two more; 0 over 30 where the config holds 30. In a hand sample about 2 of 4 hits were worth splitting. A 160-line nest scores 143 and fires; a 159-line flat table scores 0; splitting a nest into two methods that share state still fires on both. Nine commits extract a helper to get under the limit.

## Rules it serves

arch.decisions.1, arch.decisions.4, docs.links.2
