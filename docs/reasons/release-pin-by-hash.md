---
type: reason
status: current
as-of: 2026-11-19
---

# A rulebook release is pinned by the hash of its content

A project pins a rulebook release by the hash of its content, the way `go.sum` pins a module. There is no release signing in v1. The first pin and every re-pin are taken only with the user's yes.

- **The pin:** the pin holds the hash; the lock holds the digests of the release's assets.
- **The first pin and every re-pin:** `init` fetches the release, computes its content hash and shows it in the plan. The hash is pinned only after the user's yes. An upgrade shows a new hash as a re-pin and makes it only after a yes. An unpinned release is taken only with that yes, recorded in the lock.
- **What the hash says:** that nothing changed since the user's yes. It does not say who published the release, and nothing in v1 does. It holds because the fetch checks every file it hands on against `digests.txt` by digest, and refuses the release when a file differs or the tag holds a file the list does not name. The withdrawn list is the one file read from outside the pinned tree.
- **A mismatch** is refused: a release whose content does not hash to the pinned value ends "refused".
- **No signing for v1:** no signer, no key, no signature on a release or on the withdrawn list. "Signing infrastructure" stays in the spec's "not built" lists.
- **The hash:** the release's content hash is computed through `internal/digest`.
- **The withdrawn list:** it is read unsigned, from the rulebook's own repository at the pinned source and over the fetch's own transport. A withdrawal warns at status and blocks an upgrade to the release it names. It never changes anything by itself.

## Why

A signature says who published; a content hash says nothing changed since the user said yes, which is what a consumer needs and what `go.sum` gives a module. A key means a person who must keep it, a loss and rotation procedure, and a new root every time one goes wrong. For v1 the pin by hash carries the trust without any of that, and signing can be added later as a layer over it.

The unsigned withdrawn list carries a risk. A forged list can fake a withdrawal, and it can also hide one, so a user can stay on a withdrawn release with no warning; it is not a false-alarm-only risk. The risk is accepted for v1 because forging the list takes the same access as replacing a release itself (a push to the rulebook's repository, or control of the transport), and it can be fixed later if it shows up in use. [The spec](../spec.md) says both plainly.

## What was weighed instead

- **Keeping an OpenSSH signer and a published key.** It costs a key to guard, a rotation and a lost-key procedure for a claim, who published, that the pin by hash does not need for v1.
- **A signed withdrawn list with its own key.** The same key and procedure, for a list whose forging takes the same access as replacing a release.
- **Accepting an unpinned release with a warning only.** A warning nobody reads; the yes stays.

## Rules it serves

cli.config.4 (the engine holds and names no key)
