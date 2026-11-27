---
type: board
status: current
as-of: 2026-11-08
---

# Init

Init runs in any folder. In a folder that is neither a project nor a rulebook it asks one question first. The answer can be given as an argument, so that a script never waits. In a folder already pinned, or already a rulebook, it ends not ready and says so.

## The first question

```text
init

reading the folder
no pin and no rulebook file here

What is this folder?
  1  A project that follows a rulebook
  2  A rulebook
Pick one [1/2]:
```

Ends: nothing yet, the form for the answer follows.
Answered by: `gragas init --kind project` or `gragas init --kind rulebook`. The same two answers work without a terminal.

## Without a terminal and without the argument

```text
init

reading the folder
no pin and no rulebook file here

  usage   init needs to know what this folder is
    gragas init --kind project
    gragas init --kind rulebook
```

Ends: usage, exit 2.
Answered by: the argument.

## A folder that is already set up

```text
init

reading the folder
this folder is pinned to the base rulebook at 1.2.0

  not ready   already pinned
    gragas status
```

Ends: not ready, exit 4.
Answered by: nothing.
