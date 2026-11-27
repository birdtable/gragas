package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// TestTheBoardsCheckIsRedOnATreeWithNothingToRebuild holds gragas#1228's fail closed: a root with
// no Relations page in canvas/ is an error, never a pass.
func TestTheBoardsCheckIsRedOnATreeWithNothingToRebuild(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for _, d := range []string{"docs/explorer-boards/gen", "docs/explorer-boards/canvas", "explorer/core"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	node, err := os.Executable() // any program that exists: the check stops before it runs one
	if err != nil {
		t.Fatal(err)
	}
	var out, errs bytes.Buffer
	if code := runBoards(t.Context(), []string{"check", "--node", node, "-C", root}, &out, &errs); code != exitFailed {
		t.Errorf("an empty tree: exit %d, %q%q; want red", code, out.String(), errs.String())
	}
	if code := runBoards(t.Context(), []string{"check", "-C", root}, &out, &errs); code != exitUsage {
		t.Errorf("no --node: exit %d, want usage", code)
	}
}
