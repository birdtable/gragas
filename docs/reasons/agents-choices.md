---
type: reason
status: current
as-of: 2026-11-16
---

# How coding agents are kept inside the rules

These fourteen rules of the agents part cover delivery logs, guard settings, instruction files, review and tickets. Each keeps its status and carries its reason on this page.

- **agents.delivery.5** (Hard, active): A log of what is true reads as the history of the program's behaviour; a log of actions says only that someone was busy. The body is the one place the reason survives.
- **agents.guards.10** (Session, active): A rule bent once to finish a ticket is gone for every later ticket, and the person who owns the rule never learns it was in the way.
- **agents.guards.11** (Session, active): A third try of the same thing is a rerun to green, and a restart or a new baseline hides the failure instead of explaining it.
- **agents.guards.4** (Hard, active): Settings that exist only on one machine protect nothing; 0 of 11 repositories committed any. A ban on a session mode is a rule the operator has chosen to set aside, while a hook, a deny rule and the pre-push gate hold whatever the mode is.
- **agents.guards.5** (Hard, active): Project settings can be edited by the agent; only the managed layer cannot be switched off. The mode is a trade the operator makes with the hooks and the pre-push gate as the wall, so it is recorded, not banned.
- **agents.guards.7** (Hard, active): One branch build rewrote a machine-wide hook file, and 19 agents of another group ran it for about four minutes. A fix made in someone else's module inside an unrelated change is reviewed by nobody who owns it.
- **agents.guards.8** (Session, active): Agents read hostile text as their job. Not measured yet. An environment dump puts every secret of the session into the transcript at once.
- **agents.guards.9** (Session, active): Work started on a stale or red base spends the session on someone else's fault and lands a change whose proof is about another tree; the land check refuses it late, after the work is done.
- **agents.instructions.1** (Hard, active): One controlled study found no gain in success and over 20 percent more cost from longer instruction files, and every line is paid for on every run. Three instruction files measured 33,282 B, 18,492 B and 8,811 B (and 4,400 B in another repository).
- **agents.instructions.3** (Hard, active): Rules in the root file are paid for on every task and then crowd out the instruction map. Beyond the same size study, this is unmeasured.
- **agents.review.1** (Hard, active): Argued: no study supports AI review as a verdict. The project's own evidence is that review finds faults: one project's designer agent found 31 of 72 and another's reviewers 16, and a spec-against-diff lens found a missing case that tests had not.
- **agents.tickets.1** (Hard, active): A ticket's false claim about the code is the largest fault class in the logs behind this rulebook: 52 of 137 faults. On real history the check caught 11 of about 16 known name faults.
- **agents.tickets.5** (Answered, active): Practitioner consensus, not a measurement; a size ceiling is a guess and splitting beats it.
- **agents.tickets.8** (Hard, active): A fault that lives only in a chat is found again by the next person at full cost, and an id that moves breaks every comment and test that cited it.

## Rules it serves

arch.decisions.1, arch.decisions.4, docs.links.2
