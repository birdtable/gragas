---
type: reason
status: current
as-of: 2026-11-16
---

# The service part target-state choices

The service part settles how a service authorises requests, shapes its contract, reports errors, publishes events, evolves, and logs. Twenty-two rules follow from those choices. Each states its own reason, with the strength and status it has in the rulebook.

- **service.authz.1** (Hard, active): A route with no declaration is open by omission; the gap is invisible until someone calls it. One grammar is what makes permissions readable by a stranger in year five, and a grant for a subject the machine lacks is a grant that waits for a path to appear.
- **service.authz.13** (Hard, active): A token readable from a database dump is a live credential in every backup; hashing and encryption close that leak in every program that holds one.
- **service.authz.14** (Hard, active): A 403 on one item tells the caller it exists, and an action answer that carries state leaks what the read permission would have hidden.
- **service.authz.15** (Hard, active): A forwarded permission is a decision with two homes, and a header believed on any listener can be set by anyone who reaches another one.
- **service.authz.16** (Hard, active): In an estate where agents hold tokens, the person-only list is the rule that matters most, and a permission change no one can trace cannot be reviewed.
- **service.authz.3** (Hard, active): An identity named in a body is whatever the caller says it is, and a write by an unnamed system actor cannot be traced to the job that made it.
- **service.authz.7** (Hard, active): A service with its own login is a second identity system to patch forever. Hand-rolled verification is where algorithm and expiry mistakes come from, and a per-request call to the identity service makes it a point of failure for all traffic.
- **service.authz.8** (Hard, active): A missing catalogue that is read as nothing to check turns every route into an open one, and an open read is how a system leaks what it would refuse to change.
- **service.contract.1** (Hard, active): A contract written by hand drifts from the code it describes, and a generated client built from it then lies. A gate that the same change can edit is not a gate; the merge base is the copy the change cannot touch.
- **service.contract.10** (Hard, active): What a reader needs is consistency within one program's contract; an estate-wide casing would break working contracts for no reader's benefit, and an implied default lets a program skip the decision.
- **service.contract.15** (Hard, active): A generated client depends on the whole response schema, so the provider cannot tell which fields anyone reads and with no window before cutover can retire nothing. Consumer-driven contracts let the provider learn what its consumers use; the article limits the idea to one enterprise or a closed community, which is the case here. Such a test runs on the other party's rhythm of change and should raise a task more often than it breaks a build.
- **service.contract.18** (Hard, active): A provider that names its consumer has made the consumer part of its contract, and every reader after the first inherits a shape made for someone else. Rules already stop a consumer naming its provider loosely; nothing stopped the reverse.
- **service.contract.19** (Hard, active): A field named tile is a contract with one page's layout, and the page will change; meaning-only answers are what let a second reader (a CLI, an agent) use the same answer.
- **service.contract.2** (Hard, active): A silent break reaches callers that were never told. Owners of every reader found that a compatibility layer is where their bugs lived; a publisher to readers it will never see must give them notice. One rule with a declared mode stays true for both and keeps the check mechanical.
- **service.contract.20** (Hard, active): A unit in the field name is the cheapest way never to ship a number in the wrong unit, and a sentence tested against its inputs stays honest after a formula changes.
- **service.errors.2** (Hard, active): A code that changes meaning breaks every caller that branches on it, and a code in a second casing is a second vocabulary for one reader to learn.
- **service.errors.4** (Hard, active): A code the contract does not list cannot be handled by a generated client, and platform codes are the ones nobody remembers. A reader that has to look inside a 200 for an error is a reader that guesses, and one that guesses breaks.
- **service.events.4** (Hard, active): Without a common envelope a consumer cannot deduplicate by id, route by version or attribute the actor.
- **service.evolution.2** (Hard, active): Loosening later is easy and tightening is hard, so a publisher to readers it cannot change starts at full. An owner of every reader moves them together, and keeping old shapes for readers that no longer exist is where bugs live.
- **service.evolution.3** (Hard, active): Without a date the migration never finishes and near-duplicate families run on, and new services subscribe to the old one. Where the owner deploys every reader, a second stream is a compatibility layer with no reader to serve.
- **service.logs.1** (Hard, active): Unstructured lines cannot be searched or joined across services, and a line without a correlation id cannot be placed in a request. A log field renamed by accident breaks every dashboard and metric filter silently.
- **service.logs.2** (Hard, active): Logs are copied to more places than the data they come from, and a secret or a body in them outlives every other control.

## Rules it serves

arch.decisions.1, arch.decisions.4, docs.links.2
