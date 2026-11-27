---
type: spec
status: current
as-of: 2026-11-09
---

# Translations: the translations folder

A translation is text in another language. The authoritative language is the one the identity names, and the text a translation replaces stays where it is, in the rule, term, decision or source. A translation never adds a field to an entity: it is keyed by the entity, the field and the language. The index of [the format](README.md) says how to read a table.

Schema: [translations.schema.json](../../internal/host/assets/format/schema/v3/translations.schema.json)

| Field | Required | Meaning |
|---|---|---|
| `format` | yes | The format version, always 3 here |
| `translations` | yes | The translations. May be empty |
| `translations[].entity` | yes | The entity kind and its key, such as `rule:store.schemas.2` or `term:digest` |
| `translations[].field` | yes | The field translated, one of the table below |
| `translations[].language` | yes | The language of `text` |
| `translations[].text` | yes | The translated text |
| `translations[].hash` | yes | The digest of the authoritative text this translated. The translation is stale when it differs from the digest of the text now. Staleness is derived and never a field |

## The fields a translation may name

An entry names the entity as `kind:key` and the field as written here. A field of a list names one item by its index, as in `reasoning.pros[2]` or `bindings[1].remedy`. The fields are prose a reader acts on; a quotation keeps the source's own words, a locator is not prose and an owner is an identity, so none of those is translatable.

| Entity | Fields |
|---|---|
| rule | `statement`, `reasoning.definition`, `reasoning.pros[]`, `reasoning.cons[]`, `reasoning.decision`, `bindings[].remedy`, `exemptions[].reason` |
| term | `definition` |
| decision | `title`, `text` |
| source | `title` |

That is eleven fields. The code holds the table once (`format.Translatable`) and a test holds every field to its schema, where it is a string.

## Rules the fields come with

- A key, the entity with the field and the language, is written once (A65).
- The hash is the canon digest of the authoritative text as a JSON string, not of a file.
- A stale translation is a notice and never a finding; it never fails a check (S27).
- A reader in a language with no translation gets the authoritative language's text, marked as a fallback.

## Example

```yaml translations.schema.json
format: 3
translations:
  - entity: term:gate
    field: definition
    language: fr
    text: Un contrôle qui arrête un envoi quand il échoue.
    hash: sha256:e9f040762370206a21747369cc9370f28651fdf1cc0e93572bced06b19827bf9
  - entity: term:wrap
    field: definition
    language: fr
    text: Ajouter ce qui était en cours à une erreur avant de la rendre.
    hash: sha256:258d54cd5a8e4542ac88e7f282f01d52566d7e08144201512f3475283bc30386
```
