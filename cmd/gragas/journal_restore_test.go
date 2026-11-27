package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/birdtable/go-console/shell"

	"github.com/birdtable/gragas/internal/config"
	coreapi "github.com/birdtable/gragas/internal/core/api"
	hostfs "github.com/birdtable/gragas/internal/host/fs"
	"github.com/birdtable/gragas/internal/model"
)

// TestRestoreNamesTheCopyTheJournalAdapterStored: over the host's own Journal,
// a run keeps the copy of a file and closes; the line restore prints names a
// file that exists in the state folder and holds the copy's bytes. This is the
// one place the restore's naming of a copy is held to the adapter's. Risk: a
// printed line that points at nothing.
func TestRestoreNamesTheCopyTheJournalAdapterStored(t *testing.T) {
	t.Parallel()
	dir, state := t.TempDir(), filepath.Join(t.TempDir(), "state")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	ports := world(nil, config.Config{StateDir: state})
	root := opened(t, ports, dir)
	ports.Clock = fixedClock{time.Unix(1_779_000_000, 0)}
	ports.Journal = hostfs.Journal{Tree: ports.Tree, Writer: ports.Writer, Clock: ports.Clock, Dir: state}
	jr := ports.Journal
	step := func(k, detail string) {
		if err := jr.Append(t.Context(), root, model.Record{Run: "r1", Kind: k, Command: "gragas gen --apply", Detail: []byte(detail)}); err != nil {
			t.Fatal(err)
		}
	}
	const old, now = "sha256:cba06b5736faf67e54b07b561eae94395e774c517a7d910a54369e1263ccfbd4", "sha256:11507a0e2f5e69d5dfa40a62a1bd7b6ee57e6bcd85c67c9b8431b36fff21c437"
	step(model.KindIntent, "{}")
	if err := jr.Keep(root, "r1", "a.txt", []byte("old")); err != nil {
		t.Fatal(err)
	}
	step(model.KindCopy, `{"path":"a.txt","existed":true,"before":"`+old+`"}`)
	step(model.KindClose, `{"outcome":"done","file":"","files":[{"path":"a.txt","before":"`+old+`","after":"`+now+`"}]}`)
	got, err := coreapi.JournalRestore(t.Context(), ports, root, coreapi.RestoreAsk{Run: "r1", StateDir: state}, func(argv []string) (string, error) { return shell.Quote(shell.POSIX, argv) })
	if err != nil || len(got.Lines) != 1 {
		t.Fatalf("restore: %v, lines %q", err, got.Lines)
	}
	fields := strings.Fields(got.Lines[0])
	if len(fields) != 4 || fields[0] != "cp" || fields[1] != "--" || fields[3] != "a.txt" {
		t.Fatalf("the line is %q, want cp -- <copy> a.txt", got.Lines[0])
	}
	// The line is a shell line, so a path with a character outside the plain set is one quoted word.
	if b, err := os.ReadFile(strings.Trim(fields[2], "'")); err != nil || string(b) != "old" {
		t.Errorf("the named copy holds %q (%v), want the bytes the journal kept", b, err)
	}
}

// fixedClock is a Clock that tells one time.
type fixedClock struct{ at time.Time }

func (f fixedClock) Now() time.Time { return f.at }
