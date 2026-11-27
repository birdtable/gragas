---
type: reason
status: current
as-of: 2026-11-16
---

# The store part's target-state choices

The store part settles how schemas and state files are written. Three rules follow from those choices. Each states its own reason, with the strength and status it has in the rulebook.

- **store.schemas.3** (Hard, active): In one repository 64 checks and 30 strict tables exist; another has none because its index can be rebuilt. A status written in three places drifts.
- **store.state-files.2** (Hard, active): Thirteen bodies replace a whole file and twelve of them are outside the owner; 52 non-test WriteFile sites exist and 14 replace a state file in place. A helper that makes the temp file elsewhere is caught only because CreateTemp is banned too. A config rewritten by a bare write loses its mode, its owner or the link a person made, and a crash halfway leaves half a file.
- **store.state-files.8** (Lens, active): A path from a caller can escape its directory. The traversal word had no hits in the catalogue before this rule, and the security analyser's rule for variable paths fires on every ordinary call (3 hits on 3), so it cannot gate.

## Rules it serves

arch.decisions.1, arch.decisions.4, docs.links.2
