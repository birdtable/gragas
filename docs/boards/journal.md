---
type: board
status: current
as-of: 2026-11-08
---

# Journal

The journal lists the runs that wrote in this folder, newest first. A run that was cut off is marked, and the line to roll it back is the next line.

```text
journal list

reading the runs on this folder
4 runs

  run        started  command          result
  7d41c2a    10:14    upgrade          cut off
  5b09e31    09:40    agents           done
  c3f7a10    09:02    init project     done
  e82d6b4    08:55    gen              done

  done   1 run was cut off
    gragas journal recover
```

Ends: done, exit 0.
Answered by: nothing. A run id is an argument to `gragas journal restore`.
