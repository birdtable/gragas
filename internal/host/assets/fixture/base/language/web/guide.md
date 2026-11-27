# Web

This guide is prose. The engine reads only the fenced blocks and leaves every
other byte alone.

## Forms

Every field has a label the reader can see.

```gragas
anchor: forms
rules:
  - id: web.forms.1
    statement: Give every form field a label.
    strength: must
    status: active
    owner: alex
    review_by: "2027-03-01"
    origin: authored
    sources:
      - {ref: house-call, backs: rule}
    bindings:
      - tool: stylelint
        target: source
        key: field-label
        value: true
        severity: error
        rung: 2
        run_point: commit
        pair: 1
        pair_path: src/form.html
    pairs:
      - {number: 1, target: source}
  - id: web.forms.2
    statement: Say what a field is for in its label.
    strength: should
    status: active
    owner: alex
    review_by: "2027-03-01"
    origin: authored
```

## Not rules

A block that is not a gragas block is left alone.

```go
fmt.Println("not a gragas block")
```
