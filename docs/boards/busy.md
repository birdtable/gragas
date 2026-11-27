---
type: board
status: current
as-of: 2026-11-08
---

# Busy

A run that changes files takes the lock on the folder. A second run finds it held and says when the holder started, so that a person can tell a live run from a stale one.

```text
upgrade --apply

reading the lock on this folder
held by a run that started at 10:14, 6 minutes ago

  busy   another run holds this folder
    gragas journal list
```

Ends: busy, exit 3.
Answered by: nothing. Run the command again when the holder has ended.
