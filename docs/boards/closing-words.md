---
type: board
status: current
as-of: 2026-11-08
---

# Closing words

Every run ends on one of eight words and nothing else. The word comes after the work and, where nothing was written, after the dry-run footer. A word that says the person must act has a second line, indented, with the line to type. Piped, the word goes to the error stream so that the JSON on the output stays whole.

## done

```text
  done   the pin is current
```

Ends: done, exit 0.
Answered by: nothing.

## findings

```text
  findings   3 findings, 1 of them a failure
    gragas check --json
```

Ends: findings, exit 1.
Answered by: nothing.

## failed

```text
  failed   the settings file could not be read, run 7d41c2a
    gragas journal list
```

Ends: failed, exit 1.
Answered by: nothing. The run id names the record to read.

## usage

```text
  usage   upgade is not a command, did you mean upgrade
    gragas --help
```

Ends: usage, exit 2.
Answered by: nothing.

## busy

```text
  busy   another run holds this folder, started 10:14
    gragas journal list
```

Ends: busy, exit 3.
Answered by: nothing.

## not ready

```text
  not ready   no pin in this folder
    gragas init
```

Ends: not ready, exit 4.
Answered by: nothing.

## refused

```text
  refused   release 1.1.0 has a blocked entry
    gragas release plan
```

Ends: refused, exit 5.
Answered by: nothing.

## declined

```text
  declined   a plan was shown and not applied
    gragas upgrade --apply
```

Ends: declined, exit 6.
Answered by: `--apply`.

## The accessible form

With `--accessible` (or `ACCESSIBLE` set) the word is drawn with a symbol before it, in the same place and with the same sentence after it, so the word never relies on its colour. Nothing is painted and there is no banner. The symbols are one ASCII character each, no two words share one, and none is a symbol section 17 of the spec gives a status or a standing (`? ^ = x + ~ o -`), so a screen that ends on a word and lists rules never shows one character for two meanings. The spec names none for the closing words; this set is the board's choice, held by a test.

| Word | Symbol |
|---|---|
| done | `*` |
| findings | `!` |
| failed | `#` |
| usage | `/` |
| busy | `&` |
| not ready | `%` |
| refused | `<` |
| declined | `>` |

A done run reads `* done` where it read `done`, then the sentence as before.
