---
type: reference
status: current
as-of: 2026-11-08
---

# Settings

gragas reads its settings from four places, each over the ones after it: the flags, the
environment, the settings file, and the defaults (cli.config.1). `internal/config` is the one
reader, and the tables below are its own: a test fails when a key or variable in the code has no
row here, or a row says other words than the code.

## The file

The settings file is `$XDG_CONFIG_HOME/gragas/settings.yaml`, else
`$HOME/.config/gragas/settings.yaml`; on Windows, `gragas\settings.yaml` in `%LOCALAPPDATA%`
(else `%USERPROFILE%\AppData\Local`). A run with no usable home (unset, relative, not clean) is
not ready, even with an XDG variable set: the folders are go-files' appdirs, in its XDG layout on
macOS as on Linux (gragas#1234). `--config <file>` names another file in its place, and
`GRAGAS_CONFIG` does when the flag is not given: the two ways to move it (cli.config.5). The
flag is read from the command line before the rest, since the settings decide how the run looks;
a relative path is from the working folder, and nothing after `--` counts. It is one YAML
document:

```yaml
version: 1
colour: never
programs:
  git: /usr/bin/git
sandbox: container
container: {}
keep: 14d
```

- **A missing file means the defaults.** A file `--config` or `GRAGAS_CONFIG` names that does
  not exist ends the run not ready, since a mistyped name should not quietly mean the defaults.
- **The names member reads the owner's own file alone** (gragas#1629). The repository's names
  gate, `gragas-names`, takes its list from the `names-file` key of the settings file the account's
  own home gives, `$HOME/.config/gragas/settings.yaml`, where the home is the system's record of the
  account and never the variable (gragas#1958). It ends not ready (exit 4) when `GRAGAS_CONFIG` is
  set, when `HOME` (`%USERPROFILE%` on Windows) names another folder than the account's home (it says
  to unset it), when the system cannot say the account's home (a directory-service account that a
  build without cgo cannot look up; run it as an account the system holds), or when the variable that moves the config folder (`XDG_CONFIG_HOME`;
  `%LOCALAPPDATA%` on Windows) moves it from the folder the home alone gives: `$HOME/.config` is
  accepted, and so is a relative value, which is ignored. It says to unset the variable, never
  what it names. An environment the repository controls (a
  Taskfile's `env:`, a hook) could otherwise point it at a settings file of its own whose key names
  an empty list, and the gate would look and find nothing. A list that holds no name is not ready
  too, never a pass. Every other command keeps the variables.
- **A file that cannot be read ends the run failed**, never on the defaults (cli.config.10).
- **`version` is required.** This build reads version 1. A newer version ends not ready,
  naming both; an older one, or none, is refused (store.schemas.2).
- **An unknown key is refused** at its file and line, naming it and the keys there are. A key
  spelled in another case is told its spelling (cli.config.2).
- **A key is written once in a table.** A key written twice is refused at the line it is written
  again, naming the line before, even with the same value: a file that says two things is
  ambiguous, so neither is taken.
- **A path is absolute and clean**, in a key and in `GRAGAS_CONFIG` and `GRAGAS_NAMES_FILE`. So
  it never starts with a dash, never climbs with `..`, and prints on one line.
- **A URL names no secret and no ssh option.** It holds no password, only an ssh URL names a
  user, and no host or user starts with a dash, which ssh would read as an option.
- **The file names programs gragas runs, so who may write it is checked** (A48,
  cli.config.12). It is read only when this account or root owns it and neither its group nor
  others can write it, and it is a regular file. Otherwise the run is refused with the line
  that fixes it, such as `chmod go-w '/home/alex/.config/gragas/settings.yaml'`. The check is
  made on the open file, on every read, never on its folder. The tools file gets the same check,
  when it exists, whether or not a settings file does. On a system with no owner or mode bits
  (Windows), a settings file that exists ends the run not ready, so the check fails closed.
- **A setting with several implementations names one, and takes its options in a table named
  after it** (cli.config.11): `sandbox: container` with a `container:` table. A table for an
  implementation that is not the one named is refused. This build has no options for any of
  them, so a key in one is refused as unknown.

## Keys

| Key | Default | Holds | Refused when | Environment |
|---|---|---|---|---|
| `version` | none: it must be written | the settings file's version, 1 | missing, not a whole number, or below 1; a newer one is not ready, naming both versions |  |
| `language` | en | the language gragas speaks | not en, the one this build has |  |
| `colour` | auto | colour: auto, always or never. NO_COLOR, CLICOLOR_FORCE, CI and --no-color decide over it | not auto, always or never |  |
| `shell` | auto | the shell the lines to paste are written for: auto, posix, powershell or cmd, in any case. auto is the shell gragas was started from on Windows, else posix | a value that is not auto or a name of the shells this build has, naming them | `GRAGAS_SHELL` |
| `programs` | none named | a table: the absolute path of git, container, userns, macos, ssh and credential | not a table, a key that is not one of the six, a path that is not absolute and clean, or an ssh or credential path that holds a single quote |  |
| `sandbox` | none | the sandboxed level's adapter: none, container, userns or macos, with its options in a table named after it | another value, naming the ones this build has |  |
| `container` | no options | the options of sandbox: container; this build has none | not a table, any key in it, or sandbox is not container |  |
| `userns` | no options | the options of sandbox: userns; this build has none | not a table, any key in it, or sandbox is not userns |  |
| `macos` | no options | the options of sandbox: macos; this build has none | not a table, any key in it, or sandbox is not macos |  |
| `tools` | the config folder's tools.yaml | the per-machine tools file, held to the same owner and mode as this file | a path that is not absolute and clean |  |
| `tool-paths` | none | a table: per project root, written with no link in it (as pwd -P prints it; a root written through a link matches no project), a table of tool@version to the tool's absolute path | a root or path that is not absolute and clean, or a name that is not tool@version |  |
| `mirrors` | none | a table: a rulebook source to a mirror of it, each an absolute path or an https, ssh or file URL | a value that is neither, a URL with a password in it, a user on any but ssh, or a value, host or user that starts with a dash |  |
| `keep` | 30d | how long the journal's copies and the caches are kept: a count of days (d) or hours (h) | not a whole number from 1 to 9999 followed by d or h |  |
| `names-file` | none: the names check is skipped | the private file of names public text must not hold, one regular expression a line | a path that is not absolute and clean | `GRAGAS_NAMES_FILE` |
| `base-ref` | none: check in a rulebook is not ready | the branch, tag or commit check in a rulebook holds HEAD's history against, by the merge base of the two, such as origin/main | a name git could read as an option or as more than one revision: empty, longer than 255 bytes, a space or control byte, one of \~^:?*[, a start of -, / or ., a .., @{, // or /., or an end of /, . or .lock | `GRAGAS_BASE_REF` |
| `mounts` | none: a disk mounted nosuid, or noexec without nosuid, is refused | a list of the folders root mounted a disk on for this machine, each the root of a mount itself, no link, and below no folder an account but root can change; a folder on such a disk is judged by its owners in place of refused, on Linux. The settings and tools files are read with GRAGAS_MOUNTS alone | not a list, or an entry that is not an absolute, clean path | `GRAGAS_MOUNTS` |

`gragas check` reads `tools` and `tool-paths` to find the pinned linter it asks for the linter
set (gragas#991). No command reads `programs`, `sandbox`, `mirrors` or `keep` yet. Each is read
and checked now, so a file written today keeps its meaning when the command that uses it is
built.

The `signing-key` key and the `signer` program are gone: gragas signs no release, since a
release is pinned by its content hash ([release-pin-by-hash](reasons/release-pin-by-hash.md)).
A file that still holds either is refused, naming the key, why it is gone and the page that
gives the reason, so it is not read as a misspelling; remove the line.

## The tools file

The tools file says where each tool version and sandbox runtime lives on this machine (A8). It is
`<config>/gragas/tools.yaml`, or the file the `tools` key names. A person writes it, the way the
rulebook's gate shows; gragas never writes it and never installs a tool.

```yaml
version: 1
tools:
  golangci-lint@2.15.0: /opt/golangci-lint/2.15.0/golangci-lint
  eslint@9.12.0: /home/me/tools/node_modules/.bin/eslint
container:
  runtime: /usr/bin/podman
  image: registry.example/gragas/proof@sha256:<64 hex digits>
sandbox:
  userns: /usr/bin/bwrap
  macos: /usr/bin/sandbox-exec
```

- **It is read like the settings file.** Its owner and mode are checked on the open file, on every
  read, with a settings file or without (A48). A missing file means no tools. One that cannot be
  read ends the run failed. `version` is required, and this build reads version 1. A newer
  version ends the run not ready, naming both. An unknown key, a key in another case, a key
  written twice and a value of the wrong shape are each refused at the file and the line. A run
  that ends on the tools file carries nothing of either file.
- **A tool is found at the version asked, in a fixed order** (A8): the path the settings'
  `tool-paths` give it for the project, then the path the approved plugin's grant names below the project, then
  the tools file's line for `tool@version`. The first place that holds the tool at that version
  is the one. A place whose tool answers another version is passed, and the not-found line names
  it: `eslint 9.12.0 is not found; tried settings names eslint@9.11.0, adapter
  /src/node_modules/.bin/eslint (it is 9.11.1)`. A found program, and every folder above it, is
  held to the same owner and mode check before it is asked anything, whichever step named it.
  So a settings path whose program answers another version does not end the search: a grant's
  path below the project, which a commit can put there, is tried next, under the same check.
- **An image is named by its digest.** A tag can name other bytes tomorrow, so `image` is refused
  unless it ends `@sha256:` and 64 lower-case hex digits. The name before it starts with a letter
  or a digit, since the image is an argument of the container runtime and a leading `-` is an
  option, and holds no space, control character or second `@`.

| Key | Default | Holds | Refused when |
|---|---|---|---|
| `version` | none | the tools file's version, 1 | missing, not a whole number, or below 1; a newer one is not ready, naming both versions |
| `tools` | none | a table: tool@version to the absolute path of that tool's program on this machine | not a table, a name that is not tool@version, or a path that is not absolute and clean |
| `container` | none | a table: runtime, the container runtime's absolute path, and image, the image it runs as name@sha256: and its 64 hex digits | not a table, a key that is not one of the two, a path that is not absolute and clean, or an image not named by its digest |
| `sandbox` | none | a table: the absolute path of the userns and the macos sandbox programs | not a table, a key that is not one of the two, or a path that is not absolute and clean |

No command finds a tool yet: `prove` and the `tools` and `plugin list` listings are where the tools
file is first used. The sandbox adapters read `container` and `sandbox` when they are built.

## Environment

| Variable | Default | Holds | Refused when |
|---|---|---|---|
| `GRAGAS_CONFIG` | the config folder's settings.yaml | the settings file, in place of the one in the config folder | a path that is not absolute and clean; a file it names that does not exist is not ready |
| `GRAGAS_NAMES_FILE` | unset: the names-file key, else no names check | the private names file, over the names-file key | a path that is not absolute and clean; a file that cannot be read or holds a bad pattern is not ready |
| `GRAGAS_BASE_REF` | unset: the base-ref key, else check in a rulebook is not ready | the base ref check in a rulebook holds HEAD's history against, over the base-ref key | a name git could read as an option or as more than one revision: empty, longer than 255 bytes, a space or control byte, one of \~^:?*[, a start of -, / or ., a .., @{, // or /., or an end of /, . or .lock |
| `GRAGAS_SHELL` | unset: the shell key, else auto | the shell the lines to paste are written for, over the shell key: auto, posix, powershell or cmd, in any case | a value that is not auto or a name of the shells this build has, naming them |
| `GRAGAS_MOUNTS` | unset: the mounts key, else none | the folders root mounted a disk on for this machine, over the mounts key, in the system's list form (: on Unix); the one source the settings and tools files are read with | an empty entry, or an entry that is not an absolute path in its own clean form, named; a folder that is no mount's root, a link, or below a folder an account but root can change refuses the run |
| `XDG_CONFIG_HOME` | $HOME/.config | the config folder's parent | never; a relative path is ignored |
| `XDG_STATE_HOME` | $HOME/.local/state | the state folder's parent: the lock and the run log | never; a relative path is ignored |
| `XDG_CACHE_HOME` | $HOME/.cache | the cache folder's parent | never; a relative path is ignored |
| `HOME` | unset: not ready | the home folder, which the folders are under when no XDG variable says (Unix and macOS) | unset, or a path that is not absolute and clean, or holds a control character: not ready |
| `USERPROFILE` | unset: not ready | the home folder on Windows, which the folders are under when LOCALAPPDATA is unset | unset, or a path that is not absolute and clean, or holds a control character: not ready |
| `LOCALAPPDATA` | %USERPROFILE%\AppData\Local | on Windows, the folders' parent: gragas\ in it, with State and Cache under that | never; a path that is not absolute is ignored |
| `CI` | unset | set to anything: plain output, no colour, banner or question | never |
| `NO_COLOR` | unset | set to anything: no colour, over the colour key | never |
| `CLICOLOR_FORCE` | unset | set to anything: colour even into a pipe, over the colour key; NO_COLOR wins | never |
| `TERM` | unset | `dumb`: no colour | never |
| `COLUMNS` | the terminal's own width, else 80 | the width, in digits, over the terminal's own; anything else is ignored | never |
| `ACCESSIBLE` | unset | set to anything: the accessible form, no colour | never |
| `COLORTERM` | unset | truecolor or 24bit: 24-bit colour where there is colour | never |
| `PATH` | unset: an enforcer is run with none | the PATH an enforcer tool is run with; gragas finds no tool by it | never |
| `LANG` | unset: an enforcer is run with none | the LANG an enforcer tool is run with: with PATH, the whole of its environment | never |

## The files gragas keeps

Every path of gragas's own files is made by one function, `config.Path`, and every file gragas
keeps is a row of `config.Persisted`: the file, the command that writes it, whether it can be
rebuilt, and how a write lands (architecture 5.5). A test runs every command that writes over the
in-memory ports and fails on a written path the table does not hold.

## Known limits

- A refusal that a run ends on prints before the command runs, so `gragas --help` still prints
  with a broken file, and every command refuses.
