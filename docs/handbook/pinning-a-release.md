---
type: reference
status: current
as-of: 2026-11-19
---

# Pinning a rulebook release by its content hash

This page is for a project that uses a rulebook, and for the rulebook's owner. A rulebook release is not signed: there is no release signing for v1 ([why](../reasons/release-pin-by-hash.md)). A project pins a release by the hash of its content, the way `go.sum` pins a module, and the pin says that nothing changed since the person's yes.

## The first pin

- `init` fetches the release, computes the hash of its content and shows it in the plan.
- The hash is written to the pin only after a yes. An unpinned release is taken only with that yes, and the yes is recorded in the lock.
- A release whose content does not hash to the pinned value is a mismatch, and the command ends "refused".

## An upgrade

- `upgrade` fetches the new release and shows its new hash as a re-pin.
- Nothing is re-pinned without a yes.
- A hash says that the content is the same as the content you said yes to. It does not say who published it.

## The withdrawn list

- The rulebook's own repository holds a list of withdrawn releases. It is read unsigned, at the pinned source and over the fetch's own transport.
- A withdrawal warns at `status` and blocks an upgrade to the release it names. It never changes anything by itself.
- A forged list can fake a withdrawal, and it can also hide one, so you can stay on a withdrawn release with no warning. Forging it takes the same access as replacing a release itself: a push to the rulebook's repository, or the transport.

## For the rulebook's owner

- The release's content hash goes in the release's digest list. Publish the hash on the rulebook's own channel with each release, so a project can compare it with what `init` shows before it says yes.
- No release key is made, kept or handed over.
