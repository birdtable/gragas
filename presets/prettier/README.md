# Prettier preset

`base.json` is the format the explorer and every TypeScript project hold their
files to (ts.cycle.1). It is written by hand, in JSON because Prettier loads a
shared config as a module, and a project imports it with a `.prettierrc.yaml`
holding the path as one string:

    "../presets/prettier/base.json"

Each value is Prettier's own default at the pinned version (3.9.10), written
out so that a key can be cited and a change of default in a later Prettier moves
nothing unseen. The format check runs `prettier --list-different` and reads
the files, never rewriting them. `internal/preset_prettier_test.go` runs the
pinned Prettier over fixtures, one for each key.
