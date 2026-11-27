---
type: reference
status: current
as-of: 2026-11-08
for: What each screen prints
---

# The terminal boards

These boards fix what a command prints and in what order, before any command is built on them. They do not fix colour: the shared kit decides that. Every board is monospace, at most 80 columns wide, and is the text a command family shows as its transcript and the text the terminal goldens are held to.

Each screen is a `text` block followed by two lines in the same shape: `Ends:` names the closing word and its exit code, and `Answered by:` names the flag or argument that answers each question the screen asks. A screen that asks nothing says `Answered by: nothing`.

| Board | What it fixes |
|---|---|
| [Plan screen](plan-screen.md) | the sections of a plan, in order, and the yes |
| [Destructive step](destructive-step.md) | the yes and the typed name for steps that are hard to take back |
| [Closing words](closing-words.md) | the eight endings, one screen each |
| [Init](init.md) | the first question in a folder that is neither a project nor a rulebook |
| [Guard](guard.md) | the refusal of a change that skips the history |
| [Findings](findings.md) | a listing that ends on findings |
| [Busy](busy.md) | a run that finds another run holding the lock |
| [Journal](journal.md) | the listing of past runs |

The banner, the name, its row and the version with no picture beside them, is drawn only for `gragas` with no command and at help, and only where the terminal paints. Every other command starts with its own output; where nothing paints it still opens on one plain heading line, as it did before. No board shows the banner.

Every run reads the same way: a heading, what it reads, the work, the dry-run footer when nothing was written, one closing word. The closing word is one of eight: done 0, findings 1, failed 1, usage 2, busy 3, not ready 4, refused 5, declined 6. A test holds each board to this: the front matter, the width, the closing word and its exit code, and that no printed line holds a semicolon, a code name or a path into the design notes.
