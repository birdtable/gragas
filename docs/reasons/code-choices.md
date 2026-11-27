---
type: reason
status: current
as-of: 2026-11-16
---

# Why comments, config defaults and dead code are held to a rule

Three rules of the code part cover port comments, config defaults and unused exports. Each keeps its status and carries its reason on this page.

- **code.comments.4** (Hard, active): Ports are where the reader cannot read the body; a contract that is not written is guessed. A doc comment required on every export elsewhere produces filler that restates the name, which three target repositories ban.
- **code.config.1** (Hard, active): A hard-coded fallback address sends traffic to the wrong place when the configured one is missing. A default holding one installation's fact works on the author's machine and silently misbehaves on every other.
- **code.deadcode.1** (Hard, active): 27 lines of dead exports in one Go repository; 97 and 124 unused exports in two web repositories. In a reference repository the finder caught unused exports at land on two changes. (An earlier version of this rule asked for a run per GOOS because a single run called the other system's code dead; a product now holds none, so one run suffices.)

## Rules it serves

arch.decisions.1, arch.decisions.4, docs.links.2
