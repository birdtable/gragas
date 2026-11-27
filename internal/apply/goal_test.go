package apply_test

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/apply"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// diverging is a Tree that, once the record of the yes is added and read
// back, does one more thing to the tree: what another writer, a bug or a
// disk would do after every file's own read back has passed.
type diverging struct {
	port.Tree
	armed *bool
	also  func(r model.Root)
}

func (d diverging) Read(r model.Root, p string) ([]byte, error) {
	b, err := d.Tree.Read(r, p)
	if p == "history.jsonl" && !*d.armed && strings.Contains(string(b), "plan") {
		*d.armed = true
		d.also(r)
	}
	return b, err
}

// TestAFailedGoalEndsFailedWithTheRunIdAndARolledBackTree: once every file is
// written, the tree is read again and held to the plan's goal. A data file
// put back to what it held before, after its own read back, ends the run
// failed, kind goal, with its run id, rolled back to the tree before. A data
// file that moved to bytes the run never wrote, and a line after the record
// of the yes, each end it failed, kind goal, with the record open and those
// bytes kept: a roll-back never overwrites what the run did not leave, so
// the cause names the file. The control is the same plan with nothing
// planted. Risk: "done" over a tree that does not hold the plan
// (cli.commands.22), or a roll-back over another writer's bytes.
func TestAFailedGoalEndsFailedWithTheRunIdAndARolledBackTree(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		also func(w port.Writer, r model.Root) error
		// kept is the file whose other bytes stop the roll-back, "" when
		// the run rolls back.
		kept string
	}{
		"a data file put back": {also: func(w port.Writer, r model.Root) error {
			return w.Replace(r, "rules/a.yaml", []byte("status: active\n"))
		}},
		"a data file that moved": {kept: "rules/a.yaml", also: func(w port.Writer, r model.Root) error {
			return w.Replace(r, "rules/a.yaml", []byte("status: moved\n"))
		}},
		"a line after the record": {kept: "history.jsonl", also: func(w port.Writer, r model.Root) error {
			return w.Append(r, "history.jsonl", []byte(`{"n":"after"}`))
		}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			before := f.snapshot(fixturePaths()...)
			inner, done := f.p.Writer, false
			f.p.Tree = diverging{Tree: f.p.Tree, armed: &done, also: func(r model.Root) {
				if err := c.also(inner, r); err != nil {
					t.Fatal(err)
				}
			}}

			o := f.run(f.threeFiles().spec())

			if o.Word != model.Failed || o.Failure == nil || o.Failure.Kind() != apply.FailGoal || o.Failure.Run() != runID {
				t.Fatalf("ended %q %+v, cause %v, want failed, kind goal, with the run id", o.Word, o.Failure, o.Cause)
			}
			if c.kept == "" {
				if !o.RolledBack {
					t.Errorf("not rolled back: %v", o.Cause)
				}
				if after := f.snapshot(fixturePaths()...); !maps.Equal(after, before) {
					t.Errorf("the tree after\n %q\nwant it as before\n %q", after, before)
				}
				return
			}
			planted := f.content(c.kept)
			if o.RolledBack || o.Cause == nil || !strings.Contains(o.Cause.Error(), c.kept+" changed since run "+runID) {
				t.Errorf("rolled back %v, cause %v; want the record open, naming %s", o.RolledBack, o.Cause, c.kept)
			}
			if open := f.open(); len(open) != 1 || open[0] != runID {
				t.Errorf("open records %v, want %s", open, runID)
			}
			if f.content(c.kept) != planted || planted == before[c.kept] {
				t.Errorf("%s = %q, want the bytes the run did not write kept", c.kept, f.content(c.kept))
			}
		})
	}
	t.Run("the control", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		o := f.run(f.threeFiles().spec())
		wantDone(t, o)
		if len(o.Plan.Goal.Files) != 3 {
			t.Errorf("the goal holds %+v, want the three data files", o.Plan.Goal.Files)
		}
	})
}

// TestTheGoalIsTheWritePathsAndCoveredByThePlanDigest: a goal the plan
// function answers is replaced by the one the write path makes, each data
// file's digest after, an added line's from the file it adds to; and the
// digest covers it. Risk: a context that sets its own goal and so passes
// any write.
func TestTheGoalIsTheWritePathsAndCoveredByThePlanDigest(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	tp := f.plan(edit{path: "rules/a.yaml", data: "status: retired\n"}, edit{path: "state/waive.yaml", data: "- x", appendLine: true})
	s := tp.spec()
	plan := s.Plan
	s.Plan = func(ctx context.Context) (model.Plan, error) {
		p, err := plan(ctx)
		p.Goal = model.Goal{Files: []model.Input{{Path: "rules/a.yaml", Digest: "sha256:planted"}}}
		return p, err
	}

	o := f.run(s)

	wantDone(t, o)
	want := []model.Input{
		{Path: "rules/a.yaml", Digest: apply.DigestBytes([]byte("status: retired\n"))},
		{Path: "state/waive.yaml", Digest: apply.DigestBytes([]byte("waivers: []\n- x\n"))},
	}
	if got := o.Plan.Goal.Files; len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("goal %+v, want %+v", got, want)
	}
	moved := o.Plan
	moved.Goal.Files = append([]model.Input(nil), moved.Goal.Files...)
	moved.Goal.Files[0].Digest = "sha256:other"
	if d, err := apply.Digest(moved); err != nil || d == o.Plan.Digest {
		t.Errorf("a plan with another goal has the digest %q, %v: the digest does not cover the goal", d, err)
	}
}

// resumeWords are the words of a run that goes on from where it stopped.
// Rolling back is the one way back (A37); "complete" is the release's
// exception and is not here.
func resumeWords() []string { return []string{"resume", "mark", "checkpoint", "replay", "continue"} }

// resumeVocabulary is every identifier and string of src that holds one of
// resumeWords, ignoring case. Comments may say what is rejected.
func resumeVocabulary(t *testing.T, name, src string) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), name, src, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	var found []string
	hit := func(s string) {
		for _, w := range resumeWords() {
			if strings.Contains(strings.ToLower(s), w) {
				found = append(found, name+": "+s)
				return
			}
		}
	}
	ast.Inspect(file, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.Ident:
			hit(n.Name)
		case *ast.BasicLit:
			if n.Kind == token.STRING {
				s, err := strconv.Unquote(n.Value)
				if err != nil {
					s = n.Value
				}
				hit(s)
			}
		}
		return true
	})
	return found
}

// TestNoPathResumesFromAMark: no identifier or string of the write path's
// own files speaks of resuming, marks, checkpoints or replays: an
// interrupted run is rolled back, never finished from where it stopped
// (A37: two code paths to crash-inject). Planted: a resume from a mark is
// found; a comment that names the rejection is not.
func TestNoPathResumesFromAMark(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		b, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		n++
		if found := resumeVocabulary(t, name, string(b)); len(found) > 0 {
			t.Errorf("the write path speaks of going on from a stop: %q", found)
		}
	}
	if n < 5 {
		t.Fatalf("read %d files of the write path, want its five at least", n)
	}

	t.Run("a resume from a mark is found", func(t *testing.T) {
		t.Parallel()
		src := "package apply\n\nfunc resumeFrom(m int) string { return \"resumed at the last mark\" }\n"
		if found := resumeVocabulary(t, "planted.go", src); len(found) != 2 {
			t.Errorf("found %q, want the function and the string", found)
		}
	})
	t.Run("a comment that names the rejection is not", func(t *testing.T) {
		t.Parallel()
		src := "package apply\n\n// rollBack never resumes from a mark.\nfunc rollBack() {}\n"
		if found := resumeVocabulary(t, "planted.go", src); len(found) != 0 {
			t.Errorf("found %q in a comment", found)
		}
	})
}
