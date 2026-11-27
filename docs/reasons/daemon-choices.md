---
type: reason
status: current
as-of: 2026-11-16
---

# The daemon choices behind the daemon rules

These twenty-nine rules of the daemon part cover access, backups, deployment, health, lifecycle, logging, notification, outbound calls, resilience and units. Each keeps its status and carries its reason on this page.

## Access

- **daemon.access.1** (Hard, active): A bridge address is anybody, and a local bypass is the door every later surface walks through. Written as a Go test in the kit or the store test helper; no separate program.
- **daemon.access.10** (Hard, active): These are the places a reader has to guess, and a reader that guesses breaks. 404, not 403, keeps a capability list from leaking what a caller may not see; a GET that writes makes a look change what it looked at.
- **daemon.access.2** (Hard, active): Held in one program already; a browser DNS-rebinding run was not done. A local server is attacked the same way whatever language it is written in.
- **daemon.access.3** (Hard, active): Three programs have a cross-site refusal; a same-site rule leaves a measured hole on a private network where every device shares one site.
- **daemon.access.4** (Hard, active): Held in the shared door package. A core that reads a header can be steered by any caller who can set one, and one identity value is what keeps every later door under the same check.
- **daemon.access.6** (Hard, active): A daemon test reads the headers and a depguard ban covers the template package. It binds only a daemon that serves a page; no target does today, and a daemon that answers an API only declares so (daemon.lifecycle.3).
- **daemon.access.7** (Hard, active): A proxy that forwards a key forwards it to anything a caller can name, which turns a read key into whatever the other program's API allows.
- **daemon.access.8** (Hard, proposed): A privileged helper is the one place a bug becomes root. The shape is set, but the design it encodes is still proposed, and a rule ahead of its own design is premature.
- **daemon.access.9** (Hard, active): A misconfigured daemon must refuse rather than believe everybody, and a detected mode changes when the machine changes, without anyone deciding.

## Backups, deployment and health

- **daemon.backups.1** (Hard, active): A backup nobody has restored is a hope, not a backup; the record of the last good copy is what tells a person how much they would lose today.
- **daemon.deploy.1** (Hard, active): A home server that pins images never updates, which is worse than a tag plus a digest check that says what changed. Loopback plus the private network's serve makes a written stack safe by construction, and printing shared policy keeps a program from reaching past its own machine.
- **daemon.health.2** (Hard, active): Without a signal (see the systemd man page) the manager or the install check treats a daemon that has not yet listened as up, and starts dependants or keeps a broken upgrade.
- **daemon.health.3** (Hard, active): A monitor that wakes what it watches is lying about idle and breaks idle by default (daemon.lifecycle.5) silently.

## Lifecycle

- **daemon.lifecycle.3** (Hard, active): Unproven in two programs (a pairing constant and a number helper); the test is not built. No target serves a page today and each makes a page a separate program, but the rule is right for the day one does.
- **daemon.lifecycle.4** (Hard, active): A blank 200 for a missing page is invisible; held in part by the doctor command. It binds only a daemon that embeds a page; no target does today.
- **daemon.lifecycle.5** (Hard, active): Idle power is measured on a home server and on a laptop battery; a process that keeps itself awake costs watts every hour of every day. It also keeps a daemon's behaviour explainable: nothing happens unless something asked.
- **daemon.lifecycle.6** (Hard, active): A daemon that cannot start cleanly after a crash is fixed by hand at the worst moment, and nothing may be left running unwatched.
- **daemon.lifecycle.7** (Hard, active): A reader that draws a provider's answer of another shape draws wrong numbers with confidence; checking once at setup and in doctor costs nothing at run time.

## Logging, notification and outbound calls

- **daemon.logging.1** (Hard, active): A secret in a log outlives the process that wrote it, and a program that runs agents handles prompts and conversations that are as private as a secret. A log a caller can grow or forge is an attack on the disk or the record.
- **daemon.logging.3** (Hard, active): An audit record that rotates away with the journal is not an audit, and a raw tool message shown to a person is noise at best and a leak of internals at worst.
- **daemon.notify.1** (Hard, active): A notification channel is a contract nobody tests unless the list is closed and held to its document; a silent drift is found by a person who missed an alert.
- **daemon.notify.2** (Hard, active): Noise is what gets a channel muted in its second month, and a timer added only to notify costs idle power every hour for an alert that seldom comes.
- **daemon.outbound.1** (Hard, active): Each closes a real leak: a key sent on to a redirect target, a token in an address that lands in a log, a DNS rebind between check and dial, and a payload that tells a channel more than its reader may know.
- **daemon.outbound.2** (Hard, active): A channel address often is its credential, so any record that holds it leaks it. A shared public allowance spent carelessly is taken from everyone who shares it.

## Resilience and units

- **daemon.resilience.6** (Hard, active): A poll costs power every hour whether anyone looks or not, and it makes behaviour depend on a clock no one set. Reading on request keeps idle cost at zero and every read explainable.
- **daemon.resilience.7** (Hard, active): A measured budget turns a preference for quiet into a check; an unmeasured run grows by small additions until the machine never sleeps.
- **daemon.units.2** (Hard, active): A daemon with more identity than it needs turns any bug into a compromise of that identity. On a laptop a separate account is more exposure, not less, so the principle is least identity with the trust line written down.
- **daemon.units.3** (Hard, active): A daemon that dies and stays dead is an outage no one sees; the limit stops a crash loop. A restart on a clean idle exit keeps a process awake that should sleep, and a one-shot job left where the manager loads it reruns at every login.
- **daemon.units.4** (Hard, active): A root writer in a folder a person owns can be tricked by a link into writing anywhere, and a person's daemon that passes more than the person has is a privilege raise for anyone who reaches it.

## Rules it serves

arch.decisions.1, arch.decisions.4, docs.links.2
