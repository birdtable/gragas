---
type: spec
status: current
as-of: 2026-11-16
---

# Checkers: the checkers folder

Checker code is code a consumer's tools load: a plugin, an analyzer, a pattern rule or a policy that a rule's binding needs beyond a tool's own entries (S23, S3). A rulebook keeps each artifact in a folder of its own, `checkers/<name>/`, with its own tests and one declaration, `checkers/<name>/checker.yaml`, that makes it known by name and publisher. The declaration is no trust: gragas reads it and lists it, and never loads or runs the code, and the release digests it (M5). The index of [the format](README.md) says how to read a table.

An artifact is the rulebook's own, with its code and its tests in its folder, or another rulebook's, declared by reference with `from` and holding nothing but its declaration: an import brings checker code only when the importer declares it by name and publisher (S3). An overlay publishes its own checker code and never its parent's, which the parent's release lists. A name the parent carries is the parent's: the overlay holds it only by a declaration from the parent itself, by the parent's publisher, never as its own and never from an import, and a name the parent and an import both carry is a finding until the overlay declares it from the parent. A binding names the artifact its entry comes from in its `checker` field ([rules](rules.md)), the rulebook's own or one it declares by reference, so an overlay's rule is checked by its parent's checker code through a declaration from the parent.

Schema: [checker.schema.json](../../internal/host/assets/format/schema/v3/checker.schema.json)

| Field | Required | Meaning |
|---|---|---|
| `format` | yes | The format version, always 3 here |
| `name` | yes | The artifact's name, a lowercase slug equal to its folder in the checkers folder |
| `tool` | yes | The tool whose plugin, analyzer, pattern rule or policy the artifact is, as a slug |
| `publisher` | yes | Who publishes the artifact: for an import's, the publisher it has in its own rulebook |
| `tests` | no | The artifact's own tests, a path relative to its folder. The rulebook's own artifact has them, and one with none, or with a path that holds nothing, is a finding; an import's artifact gives none |
| `licence` | no | The artifact's licence, as a person writes it |
| `from` | no | The rulebook name of the import whose artifact this declares by reference, one the identity imports; absent for the rulebook's own |

## What validate finds

- a file under `checkers/<name>/` where no declaration stands, or a file straight in the checkers folder;
- a declaration it cannot read, at its line: it fails closed;
- the rulebook's own artifact with no tests, or with tests that hold nothing; an import's, declared by reference, whose folder holds code;
- an import's checker code the rulebook does not declare by its name and publisher, a declaration from a rulebook it does not import, and one from an import that carries no such artifact;
- an overlay's declaration by a name its parent carries, its own or one from an import, unless it is from the parent by the parent's publisher;
- a name more than one carries, as one finding: the parent and an import, the rulebook's own artifact and an import's, or two imports. A rulebook holds one artifact by a name, so the fix keeps one: the parent's, declared from the parent; the rulebook's own, renamed, with the import's declared by reference in the folder it leaves; or one import's, the others dropped. Where the rulebook's own code sits under the name, declared, undeclared or under a refused declaration, the fix never declares anything by reference there, from an import or from the parent: yours is renamed, or theirs dropped, and a refused declaration is written first;
- an empty publisher or licence, one that prints as nothing, such as one Hangul filler, or one with a control character, a format character such as a zero-width space or a right-to-left override, or a line or paragraph separator, which the reader refuses whatever the schema says;
- a binding whose `checker` names no declaration, or one of another tool.

A parent's and an import's checker code are read from them once they are fetched (#234). Until then nothing is fetched, so on a live run neither the import's half nor the overlay's rule against its parent is checked; a declaration by reference, the rulebook's own file, is read and listed now. No fix sends the author back to the finding they came from.

## Example

```yaml checker.schema.json
format: 3
name: wrapcheck-plugin
tool: golangci-lint
publisher: alex
tests: testdata
licence: Apache-2.0
```
