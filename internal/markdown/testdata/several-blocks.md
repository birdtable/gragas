# Go errors

Intro paragraph that
runs two lines.

## Wrapping

A caller MUST wrap an error with context.

```gragas
id: go.errors.1
rule: wrap errors
```

## Wrapping

```gragas
id: go.errors.2
```

```go
fmt.Println("MUST not count")
```
