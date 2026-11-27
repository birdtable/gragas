---
type: reason
status: current
as-of: 2026-11-16
---

# Adopt a gate only when it can start frozen

Four rules share one practice: a check is adopted in an order that lets it start from what exists today and tighten over time. A ticket's claims about the code are checked before work starts, every gate has a plant that proves it fires, and a new rule starts frozen at the current count and ratchets down toward zero.

Each rule below carries its own reason on this page.

- **agents.tickets.1** (Hard, active): A ticket's false claim about the code is the largest fault class in the logs behind this rulebook: 52 of 137 faults. On real history the check caught 11 of about 16 known name faults.
- **gates.plants.1** (Hard, active): No Go repository plants a fault for each of its gates. One repository's drift check failed 13 of 35 entries and was not part of its `make check`. One web repository's 82 plants never ran in `make check` because the layer check ran without its self-test flag. Another has 85 plants on its main branch (135 on a branch). A self-test run is held in one repository (73 plants).
- **gates.ratchets.1** (Hard, active): The practice is held in one Go repository (ratchet files, no plant recorded) and in two web repositories (numeric baselines with a switch that allows them to grow). A new rule that cannot start frozen is never adopted.
- **gates.ratchets.4** (Hard, active): A ratchet that never reaches zero is a permanent exemption. Whether the baselines have shrunk was never measured. A reference team's countdowns to zero paid off.

## Rules it serves

arch.decisions.1, arch.decisions.4, docs.links.2
