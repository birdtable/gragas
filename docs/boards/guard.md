---
type: board
status: current
as-of: 2026-11-16
---

# Guard

The guard is what the rulebook's hook and gate call. `guard edit` judges one edit an agent is about to make, as the agent's hook hands it on standard input, and refuses it when it changes a lifecycle field by hand. `guard staged` judges the change staged for a commit, as the rulebook's git hook runs it: HEAD and the index, read from git and never from the working folder. It makes each history line the change appends again, its command planned over the tree before it, and refuses a lifecycle field with no line, a line with no change, a forged digest and a rewritten line. With none, each ends done.

## An edit that changes a status by hand

The agent's hook runs `gragas guard edit || exit 2`, so a refusal, or a guard that is not there, blocks the edit. The refusal and the line to type are also written to standard error, where the hook hands them to the agent. A refusal names the first field. The table lists every one, each with the command that changes it with its history entry.

```text
guard edit

Lifecycle fields this edit changes by hand
about             field   before    after   the command
rule:go.errors.2  status  proposed  active  gragas rule promote go.errors.2

  refused    rules/errors.yaml changes status of rule:go.errors.2 by hand
    gragas rule promote go.errors.2
```

Ends: refused, exit 5.
Answered by: nothing. The line to type makes the change with its history entry.

## An edit of prose

```text
guard edit

  done       rules/errors.yaml changes no lifecycle field
```

Ends: done, exit 0.
Answered by: nothing. The hook lets the edit through.

An input the guard cannot read, a file outside the repository and a fault of the guard's own each end refused too, never done. So do these, on every host:
- a name, of the file or a folder above it, as the edit spells it and as the host finds it, that folds to one the repository or the loader has and is spelled otherwise, whether or not the file is there;
- a new name some host reads as another: anything but printable ASCII, a character Windows reads as more than a name, an end in a dot or a space, or a ~ and a digit;
- an Edit that takes text out before a line's end with nothing in its place, since the agent's host takes the end out too;
- an edit that leaves a file with findings it had before, since the guard cannot read what changes inside it.

Outside a rulebook the guard ends not ready.

## A staged change that skips the history (guard staged)

The hook runs `gragas guard staged || exit 2`, so a refusal, or a guard that is not there, stops the commit. The pre-merge-commit hook runs `gragas guard staged --merge-hook || exit 2`, so a merge git commits itself, with no conflict, is judged too: git runs that hook before it records the merge, so a merge that changes a lifecycle field or `history.jsonl` is refused there with `git commit` as the line to type, and the pre-commit hook judges it when it is committed. The refusal and the line to type are also written to standard error. The table lists every lifecycle field the staged change holds otherwise than its history lines leave it, each with the command that changes it with its line.

```text
guard staged

Lifecycle fields the staged change does not record
about             field   before    after   the command
rule:go.errors.2  status  proposed  active  gragas rule promote go.errors.2

  refused    status of rule:go.errors.2 is staged with no line of history.jsonl
    gragas rule promote go.errors.2
```

Ends: refused, exit 5.
Answered by: nothing. Unstage the field, and the line to type makes the change with its history line.

## A staged line that is not its command's (guard staged)

Each line the change appends is planned again from its `command`, as its `who` at its `when`, over HEAD's tree and the lines before it. Each of these ends refused, exit 5, with the line to type beside it:
- a plan whose digest is not the line's, the refusal naming both digests;
- a file the commands write that is staged otherwise, such as a hand edit staged with a command's change;
- a line staged without the change it records;
- a line HEAD has, removed, rewritten or moved.

During a merge, a line the other side's history holds is the other side's, and so is a field it changes from the merge base; anything else ends refused. A change it cannot read or make again ends refused too, never done, and each refusal is kept in the journal.

## A staged change that is covered (guard staged)

```text
guard staged

  done       every lifecycle field staged is recorded in history.jsonl
```

Ends: done, exit 0.
Answered by: nothing. The hook lets the commit through.
