package porttest_test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/port"
	"github.com/birdtable/gragas/internal/port/contracttest"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// recorder is the Reporter a test of the fake hands it, so the failure the
// fake reports is read and not raised.
type recorder struct{ said []string }

func (r *recorder) Helper() {}

func (r *recorder) Errorf(format string, args ...any) {
	r.said = append(r.said, fmt.Sprintf(format, args...))
}

// said is whether the one failure the fake reported names want.
func (r *recorder) saidOnly(t *testing.T, want ...string) {
	t.Helper()
	if len(r.said) != 1 {
		t.Fatalf("the fake reported %d failures, want one: %q", len(r.said), r.said)
	}
	for _, w := range want {
		if !strings.Contains(r.said[0], w) {
			t.Errorf("the failure %q does not name %q", r.said[0], w)
		}
	}
}

// wordCount is a trivial core function: it reads a file through a Tree and
// counts its words. It knows no file system.
func wordCount(tree port.Tree, dir, file string) (int, error) {
	root, err := tree.Open(dir)
	if err != nil {
		return 0, err
	}
	b, err := tree.Read(root, file)
	if err != nil {
		return 0, err
	}
	return len(strings.Fields(string(b))), nil
}

// TestACoreFunctionRunsOnAMapBackedTree: with nothing but the fake, a core
// function reads a file that no disk holds. Planted: the same call on a root
// nobody seeded fails.
func TestACoreFunctionRunsOnAMapBackedTree(t *testing.T) {
	t.Parallel()
	w := porttest.NewWorld(t)
	w.Seed("/book", map[string]string{"notes.txt": "one two three\n"})
	if n, err := wordCount(w.Ports().Tree, "/book", "notes.txt"); err != nil || n != 3 {
		t.Errorf("wordCount = %d, %v, want 3", n, err)
	}
	rec := &recorder{}
	if _, err := wordCount(porttest.NewWorld(rec).Ports().Tree, "/book", "notes.txt"); !errors.Is(err, porttest.ErrUnseeded) {
		t.Errorf("a core function on an unseeded root = %v, want ErrUnseeded", err)
	}
	rec.saidOnly(t, "Tree.Open")
}

// TestTheFakesViewLeavesOutAFileBehindALink: a file seeded at a path a link
// also covers is not in FS's view, as host/fs refuses a link at any step. The
// control: the file beside it, behind no link, is in the view.
func TestTheFakesViewLeavesOutAFileBehindALink(t *testing.T) {
	t.Parallel()
	w := porttest.NewWorld(t)
	root := w.Seed("/book", map[string]string{"linked/notes.txt": "behind\n", "plain/notes.txt": "beside\n", "real/notes.txt": "real\n"})
	w.Link(root, "linked", "real")
	view, err := w.Ports().Tree.FS(root)
	if err != nil {
		t.Fatalf("FS = %v", err)
	}
	if b, err := fs.ReadFile(view, "linked/notes.txt"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the view read linked/notes.txt = %q, %v, want fs.ErrNotExist: the link was followed", b, err)
	}
	if b, err := fs.ReadFile(view, "plain/notes.txt"); err != nil || string(b) != "beside\n" {
		t.Errorf("the view read plain/notes.txt = %q, %v, want %q", b, err, "beside\n")
	}
}

// TestTheFakeReturnsNoConstant: what the fake makes comes from what it was
// given. Two commits of different content have different names, the same
// content in two roots another, and a lock's time is the seeded instant.
func TestTheFakeReturnsNoConstant(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w := porttest.NewWorld(t)
	a := w.Seed("/a", map[string]string{"f": "one"})
	b := w.Seed("/b", map[string]string{"f": "one"})
	p := w.Ports()
	c1, err := p.Remote.Commit(ctx, a, []string{"f"}, "first")
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Writer.Replace(a, "f", []byte("two")); err != nil {
		t.Fatal(err)
	}
	c2, err := p.Remote.Commit(ctx, a, []string{"f"}, "first")
	if err != nil {
		t.Fatal(err)
	}
	c3, err := p.Remote.Commit(ctx, b, []string{"f"}, "first")
	if err != nil {
		t.Fatal(err)
	}
	if c1 == c2 || c1 == c3 || c2 == c3 || len(c1) != 40 {
		t.Errorf("commits %q, %q and %q are not three distinct names of forty digits", c1, c2, c3)
	}
	for _, at := range []time.Time{time.Date(2026, 11, 8, 6, 0, 0, 0, time.UTC), time.Date(2027, 1, 2, 3, 4, 5, 0, time.UTC)} {
		w.Instant(at)
		h, err := p.Lock.Take(ctx, a, model.RootLock)
		if err != nil || h.Since != at.Unix() {
			t.Errorf("a lock taken at %v has Since %d, %v", at, h.Since, err)
		}
		if err := p.Lock.Release(h); err != nil {
			t.Fatal(err)
		}
	}
}

// TestTheWriterRecordsAndFailsOnDemand: a replace and an append change what the
// tree reads and are recorded; a failure the test asked for leaves it as it
// was.
func TestTheWriterRecordsAndFailsOnDemand(t *testing.T) {
	t.Parallel()
	w := porttest.NewWorld(t)
	root := w.Seed("/r", map[string]string{"log": "torn"})
	p := w.Ports()
	if err := p.Writer.Append(root, "log", []byte("next")); err != nil {
		t.Fatal(err)
	}
	if got, _ := w.File(root, "log"); got != "torn\nnext\n" {
		t.Errorf("the log holds %q, want the torn line ended and the new one added", got)
	}
	if err := p.Writer.Append(root, "log", []byte("a\n\nb")); err == nil {
		t.Error("an append with an empty line between two was taken")
	}
	boom := errors.New("the disk is full")
	w.WriteFails("cfg", boom)
	if err := p.Writer.Replace(root, "cfg", []byte("x")); !errors.Is(err, boom) {
		t.Errorf("a write made to fail = %v, want the planted error", err)
	}
	if _, ok := w.File(root, "cfg"); ok {
		t.Error("a failed write left a file")
	}
	if err := p.Writer.Replace(root, "../out", nil); !errors.Is(err, port.ErrOutside) {
		t.Errorf("a write that leaves the root = %v, want ErrOutside", err)
	}
	if len(w.Writes) != 1+0 && len(w.Writes) != 1 {
		t.Errorf("the writer recorded %d writes, want the one append", len(w.Writes))
	}
}

// TestALockIsHeldOrFree: a lock another holds is ErrHeld, ours is held until
// released, and a release of one not held is refused.
func TestALockIsHeldOrFree(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w := porttest.NewWorld(t)
	root := w.Seed("/r", nil)
	w.Instant(time.Date(2026, 11, 8, 6, 0, 0, 0, time.UTC))
	w.HoldLock(root, model.ScratchLock, 1234)
	p := w.Ports()
	if _, err := p.Lock.Take(ctx, root, model.ScratchLock); !errors.Is(err, port.ErrHeld) || !sinceIs(err, 1234) {
		t.Errorf("a lock held elsewhere = %v, want ErrHeld naming since 1234", err)
	}
	h, err := p.Lock.Take(ctx, root, model.RootLock)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Lock.Take(ctx, root, model.RootLock); !errors.Is(err, port.ErrHeld) {
		t.Errorf("a second take = %v, want ErrHeld", err)
	}
	if err := p.Lock.Release(h); err != nil {
		t.Error(err)
	}
	if err := p.Lock.Release(h); err == nil {
		t.Error("a lock released twice was taken")
	}
	if err := p.Lock.Release(model.Hold{}); err == nil {
		t.Error("the zero hold was released")
	}
}

// TestTheJournalKeepsWhatItIsGiven: records come back in order, a record with
// no run or an unknown kind is refused, a copy is kept by run and path, and
// torn lines are the count the test seeded.
func TestTheJournalKeepsWhatItIsGiven(t *testing.T) {
	t.Parallel()
	w := porttest.NewWorld(t)
	root := w.Seed("/r", nil)
	w.TornLines(root, 2)
	j := w.Ports().Journal
	for _, r := range []model.Record{{Run: "r1", Kind: "intent", Detail: []byte(`{"a":1}`)}, {Run: "r1", Kind: "close"}} {
		if err := j.Append(t.Context(), root, r); err != nil {
			t.Fatal(err)
		}
	}
	got, torn, err := j.Records(t.Context(), root)
	if err != nil || torn != 2 || len(got) != 2 || got[0].Kind != "intent" || got[1].Kind != "close" {
		t.Errorf("Records = %+v, %d, %v", got, torn, err)
	}
	for _, bad := range []model.Record{{Kind: "intent"}, {Run: "r", Kind: "novel"}} {
		if err := j.Append(t.Context(), root, bad); err == nil {
			t.Errorf("the record %+v was taken", bad)
		}
	}
	if err := j.Keep(root, "r1", "f", []byte("old")); err != nil {
		t.Fatal(err)
	}
	if c, err := j.Kept(root, "r1", "f"); err != nil || string(c) != "old" {
		t.Errorf("Kept = %q, %v", c, err)
	}
	if _, err := j.Kept(root, "r2", "f"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Kept of another run = %v, want fs.ErrNotExist", err)
	}
}

// TestHistoryServesFixedTreesPerTag: a tag, a commit and the index each give
// the tree the test seeded, and nothing else.
func TestHistoryServesFixedTreesPerTag(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w := porttest.NewWorld(t)
	root := w.Seed("/r", nil)
	who := model.Identity{Name: "A Person", Email: "a@example.com"}
	w.SeedRepo(root, porttest.Repo{
		Tags: map[string]porttest.Rev{
			"v1": {Commit: "aaaa", Files: map[string]string{"a": "1"}},
			"v2": {Commit: "bbbb", Files: map[string]string{"a": "2", "b": "x"}},
		},
		Index: map[string]string{"a": "3"},
		Who:   &who,
		Bases: map[[2]string]string{{"v1", "v2"}: "aaaa"},
	})
	h := w.Ports().History
	if got, err := h.Blob(ctx, root, "v1", "a"); err != nil || string(got) != "1" {
		t.Errorf("Blob(v1) = %q, %v", got, err)
	}
	if got, err := h.Blob(ctx, root, "bbbb", "b"); err != nil || string(got) != "x" {
		t.Errorf("Blob(commit) = %q, %v", got, err)
	}
	if got, err := h.Blob(ctx, root, "", "a"); err != nil || string(got) != "3" {
		t.Errorf("Blob(index) = %q, %v", got, err)
	}
	if _, err := h.Blob(ctx, root, "v1", "b"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a path the rev lacks = %v, want fs.ErrNotExist", err)
	}
	if _, err := h.Files(ctx, root, "v9"); err == nil {
		t.Error("a rev that does not exist was taken")
	}
	files, err := h.Files(ctx, root, "v2")
	if err != nil || len(files) != 2 || files[0].Path != "a" || files[1].Path != "b" {
		t.Errorf("Files(v2) = %+v, %v", files, err)
	}
	tags, err := h.Tags(ctx, root)
	if err != nil || len(tags) != 2 || tags[0].Name != "v1" || tags[1].Commit != "bbbb" {
		t.Errorf("Tags = %+v, %v", tags, err)
	}
	if got, err := h.Who(ctx, root); err != nil || got != who {
		t.Errorf("Who = %+v, %v", got, err)
	}
	if got, err := h.Base(ctx, root, "v2", "v1"); err != nil || got != "aaaa" {
		t.Errorf("Base = %q, %v, want it either way round", got, err)
	}
}

// TestRemoteFetchesFromAMapAndRecordsWhatItWrites: a seeded release is a root
// the tree reads, a commit with nothing new is refused, and a tag that exists
// is left alone.
func TestRemoteFetchesFromAMapAndRecordsWhatItWrites(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w := porttest.NewWorld(t)
	w.SeedRelease("example.com/book", "v1", map[string]string{"rules.yaml": "r: 1"})
	p := w.Ports()
	root, err := p.Remote.Fetch(ctx, "example.com/book", "v1")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := p.Tree.Read(root, "rules.yaml"); err != nil || string(got) != "r: 1" {
		t.Errorf("the fetched root reads %q, %v", got, err)
	}
	if _, err := p.Remote.Fetch(ctx, "example.com/book", "v9"); err == nil {
		t.Error("a tag that is not there was fetched")
	}
	if _, err := p.Remote.Commit(ctx, root, []string{"rules.yaml"}, "add"); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Remote.Commit(ctx, root, []string{"rules.yaml"}, "again"); err == nil {
		t.Error("a commit with no change was made")
	}
	if err := p.Remote.Tag(ctx, root, "v1", "abc", "release"); err != nil {
		t.Fatal(err)
	}
	if err := p.Remote.Tag(ctx, root, "v1", "abc", "release"); err == nil {
		t.Error("a tag that exists was made again")
	}
	if len(w.Commits) != 1 || len(w.Tags) != 1 {
		t.Errorf("recorded %d commits and %d tags, want one each", len(w.Commits), len(w.Tags))
	}
}

// TestTheSandboxReachesWhatItIsAsked: the level asked is the level reached, a
// lower one when the test says the platform has no more, and ErrUnavailable
// when it has none; the folder holds the fixtures and goes with Release.
func TestTheSandboxReachesWhatItIsAsked(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w := porttest.NewWorld(t)
	p := w.Ports()
	s, err := p.Sandbox.Prepare(ctx, model.Prepare{Level: model.Sandboxed, Fixtures: []model.File{{Path: "f.txt", Data: []byte("x")}}})
	if err != nil || s.Level != model.Sandboxed {
		t.Fatalf("Prepare = %+v, %v", s, err)
	}
	root, err := p.Tree.Open(s.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := p.Tree.Read(root, "f.txt"); err != nil || string(got) != "x" {
		t.Errorf("the folder holds %q, %v", got, err)
	}
	s2, _ := p.Sandbox.Prepare(ctx, model.Prepare{Level: model.Isolated})
	if s2.Dir == s.Dir {
		t.Error("two folders have one name")
	}
	if err := p.Sandbox.Release(s); err != nil {
		t.Error(err)
	}
	w.SandboxReaches(model.Sandboxed, model.Isolated)
	if s, err := p.Sandbox.Prepare(ctx, model.Prepare{Level: model.Sandboxed}); err != nil || s.Level != model.Isolated {
		t.Errorf("a platform with no more = %+v, %v, want isolated", s, err)
	}
	w.SandboxMissing(model.Isolated)
	if _, err := p.Sandbox.Prepare(ctx, model.Prepare{Level: model.Isolated}); !errors.Is(err, port.ErrUnavailable) {
		t.Errorf("a level the platform lacks = %v, want ErrUnavailable", err)
	}
}

// TestTheSchemaIsValidUnlessAFindingIsPlanted: planted for one document and
// not another.
func TestTheSchemaIsValidUnlessAFindingIsPlanted(t *testing.T) {
	t.Parallel()
	w := porttest.NewWorld(t)
	w.SeedSchemas("s.json")
	w.Plant("s.json", []byte(`{}`), model.Problem{Line: 3, Msg: "c"}, model.Problem{Line: 1, Msg: "a"})
	s := w.Ports().Schema
	got, err := s.Check("s.json", []byte(`{}`))
	if err != nil || len(got) != 2 || got[0].Line != 1 {
		t.Errorf("a planted finding = %+v, %v, want two sorted by line", got, err)
	}
	if got, err := s.Check("s.json", []byte(`{"id":1}`)); err != nil || len(got) != 0 {
		t.Errorf("a document nothing was planted for = %+v, %v, want valid", got, err)
	}
}

// TestTheContractSuitesPassOnTheFake: each suite of internal/port/contracttest
// runs against the fake. The real adapters run the same suites in their own
// packages as they are built; the Clock, Assets and Terminal adapters come in
// M0a-16 to M0a-18, and the file adapter in M0b.
func TestTheContractSuitesPassOnTheFake(t *testing.T) {
	t.Parallel()
	t.Run("Sandbox", func(t *testing.T) {
		t.Parallel()
		contracttest.Sandbox(t, func(t *testing.T) contracttest.SandboxSetup {
			t.Helper()
			w := porttest.NewWorld(t)
			return contracttest.SandboxSetup{
				Sandbox: w.Ports().Sandbox,
				Files:   w.Scratch,
				Read: func(s model.Scratch, rel string) ([]byte, error) {
					if c, ok := w.File(model.Root(s.Dir), rel); ok {
						return []byte(c), nil
					}
					return nil, fs.ErrNotExist
				},
			}
		})
	})
	t.Run("Clock", func(t *testing.T) {
		t.Parallel()
		seeded := time.Date(2026, 11, 8, 6, 0, 0, 0, time.FixedZone("elsewhere", 3600))
		contracttest.Clock(t, func(t *testing.T) port.Clock {
			t.Helper()
			w := porttest.NewWorld(t)
			w.Instant(seeded)
			return w.Ports().Clock
		}, func() time.Time { return seeded })
	})
	t.Run("Assets", func(t *testing.T) {
		t.Parallel()
		known := map[string]string{"handbook/a.md": "a", "profiles/p.yaml": "p: 1\n", "empty": ""}
		contracttest.Assets(t, known, func(t *testing.T) port.Assets {
			t.Helper()
			w := porttest.NewWorld(t)
			w.SeedAssets(known)
			return w.Ports().Assets
		})
	})
	t.Run("Schema", func(t *testing.T) {
		t.Parallel()
		contracttest.Schema(t, func(t *testing.T, c contracttest.SchemaCase) port.Schema {
			t.Helper()
			w := porttest.NewWorld(t)
			w.SeedSchemas(c.Name)
			w.Plant(c.Name, c.Bad, model.Problem{Line: 1, Msg: "id is required"})
			return w.Ports().Schema
		})
	})
	t.Run("Journal", func(t *testing.T) {
		t.Parallel()
		contracttest.Journal(t, func(t *testing.T) contracttest.JournalSetup {
			t.Helper()
			w := porttest.NewWorld(t)
			now := time.Date(2026, 11, 8, 6, 0, 0, 0, time.UTC)
			w.Instant(now)
			pass := func(d time.Duration) { now = now.Add(d); w.Instant(now) }
			// The suite holds where a copy is kept to this system's idea
			// of an absolute, clean path.
			w.KeepCopiesIn(t.TempDir())
			root := w.Seed("/work/a", nil)
			return contracttest.JournalSetup{J: w.Ports().Journal, Root: root, Other: w.Seed("/work/b", nil), Pass: pass,
				Bytes: func() []byte { return w.JournalBytes(root) }}
		})
	})
	t.Run("Lock", func(t *testing.T) {
		t.Parallel()
		contracttest.Lock(t, func(t *testing.T) contracttest.LockSetup {
			t.Helper()
			w := porttest.NewWorld(t)
			w.Instant(time.Date(2026, 11, 8, 6, 0, 0, 0, time.UTC))
			root, other := w.Seed("/work/a", nil), w.Seed("/work/b", nil)
			l := w.Ports().Lock
			return contracttest.LockSetup{A: l, B: l, Root: root, Other: other, Since: porttest.HeldSince}
		})
	})
	t.Run("Trust", func(t *testing.T) {
		t.Parallel()
		contracttest.Trust(t, func(t *testing.T) contracttest.TrustSetup {
			t.Helper()
			w := porttest.NewWorld(t)
			carried := porttest.NewWorld(t)
			carried.SeedTrustRefused(fmt.Errorf("the state folder lies in the project: %w", port.ErrCarried))
			x := w.Ports().Trust
			return contracttest.TrustSetup{A: x, B: x, Carried: carried.Ports().Trust, Refuse: func(*testing.T) error {
				err := fmt.Errorf("the record's group can write it: %w: %w", port.ErrNotOwned, port.ErrOwnedWritable)
				w.SeedTrustRefused(err)
				return port.ErrOwnedWritable
			}}
		})
	})
	t.Run("Steps", func(t *testing.T) {
		t.Parallel()
		contracttest.Steps(t, func(t *testing.T, note func(context.Context, plugin.Noted) error) (port.Steps, plugin.Scope) {
			t.Helper()
			w := porttest.NewWorld(t)
			w.SeedTool("lint-a", model.Found{Path: "/fake/lint-a", Version: "1.2.3"})
			m := plugin.Manifest{Tool: "lint-a", Programs: []plugin.Program{{Found: &plugin.ProgramFound{ID: "lint-a", Versions: ">=1.0.0 <2.0.0", VersionArgv: []string{"--version"}}}}}
			return w.Ports().Steps, plugin.Scope{Manifest: m, Root: "/work/a", Floor: plugin.LevelIsolated, Note: note,
				Scratch: model.Scratch{Dir: "/scratch/a", Level: model.Isolated, Seconds: 60}}
		})
	})
	t.Run("Terminal", func(t *testing.T) {
		t.Parallel()
		contracttest.Terminal(t, func(t *testing.T, lines []string) port.Terminal {
			t.Helper()
			w := porttest.NewWorld(t)
			w.SeedTerminal(porttest.Term{Answers: lines, Input: lines})
			return w.Ports().Terminal
		})
	})
	t.Run("FileNames", func(t *testing.T) {
		t.Parallel()
		contracttest.FileNames(t, func(t *testing.T) port.FileNames {
			t.Helper()
			return porttest.NewWorld(t).Ports().FileNames
		})
	})
	t.Run("Tree", func(t *testing.T) {
		t.Parallel()
		contracttest.Tree(t, func(t *testing.T, files map[string]string) (port.Tree, string, contracttest.Outside) {
			t.Helper()
			w := porttest.NewWorld(t)
			w.Seed("/work/tree", files)
			// The suite reaches the file outside from the working folder,
			// so it is at this system's absolute path, not one the world
			// spells.
			dir := t.TempDir()
			outside := contracttest.Outside{File: filepath.Join(dir, "outside.txt")}
			w.SeedOutside(outside.File, "outside\n")
			return w.Ports().Tree, "/work/tree", outside
		})
	})
	t.Run("Owned", func(t *testing.T) {
		t.Parallel()
		contracttest.Owned(t, func(t *testing.T) (port.Owned, contracttest.Outside) {
			t.Helper()
			w := porttest.NewWorld(t)
			dir := t.TempDir()
			outside := contracttest.Outside{File: filepath.Join(dir, "outside.txt"), Refused: filepath.Join(dir, "loose.txt")}
			w.SeedOutside(outside.File, "outside\n")
			w.SeedOutside(outside.Refused, "outside\n")
			w.SeedRefused(outside.Refused, porttest.Refused{Why: "is writable by its group", Fix: "chmod g-w " + outside.Refused})
			return w.Ports().Owned, outside
		})
	})
	t.Run("Writer", func(t *testing.T) {
		t.Parallel()
		contracttest.Writer(t, func(t *testing.T, files map[string]string) (port.Tree, port.Writer, string) {
			t.Helper()
			w := porttest.NewWorld(t)
			w.Seed("/work/tree", files)
			p := w.Ports()
			return p.Tree, p.Writer, "/work/tree"
		})
	})
	t.Run("Links", func(t *testing.T) {
		t.Parallel()
		contracttest.Links(t, func(t *testing.T, files, links map[string]string) (port.Tree, port.Writer, string) {
			t.Helper()
			w := porttest.NewWorld(t)
			root := w.Seed("/work/tree", files)
			for p, target := range links {
				w.Link(root, p, target)
			}
			p := w.Ports()
			return p.Tree, p.Writer, "/work/tree"
		})
	})
	t.Run("Names", func(t *testing.T) {
		t.Parallel()
		contracttest.Names(t, func(t *testing.T, files, names map[string]string) (port.Tree, string) {
			t.Helper()
			w := porttest.NewWorld(t)
			root := w.Seed("/work/tree", files)
			for second, first := range names {
				w.Name(root, first, second)
			}
			return w.Ports().Tree, "/work/tree"
		})
	})
	t.Run("Who", func(t *testing.T) {
		t.Parallel()
		contracttest.Who(t, func(t *testing.T, who *model.Identity) (contracttest.Identifier, model.Root) {
			t.Helper()
			w := porttest.NewWorld(t)
			w.SeedRepo("/work/book", porttest.Repo{Who: who})
			return w.Ports().History, "/work/book"
		})
	})
	t.Run("History", func(t *testing.T) {
		t.Parallel()
		contracttest.History(t, func(t *testing.T, f contracttest.HistoryFixture) (port.History, model.Root) {
			t.Helper()
			w := porttest.NewWorld(t)
			w.SeedRepo("/work/book", repoOf(f))
			seedWork(w, "/work/book", f)
			return w.Ports().History, "/work/book"
		})
		w := porttest.NewWorld(t)
		w.SeedRepo("/work/none", porttest.Repo{})
		contracttest.NoTags(t, w.Ports().History, "/work/none")
	})
	t.Run("Remote", func(t *testing.T) {
		t.Parallel()
		contracttest.Remote(t, func(t *testing.T) contracttest.RemoteSetup {
			t.Helper()
			w := porttest.NewWorld(t)
			root := w.Seed("/work/book", map[string]string{"rules.yaml": "one\n"})
			w.SeedRelease("/src/book", "v1", contracttest.TheRelease())
			p := w.Ports()
			return contracttest.RemoteSetup{
				R: p.Remote, Root: root, From: "/src/book",
				Write: func(path, c string) {
					if err := p.Writer.Replace(root, path, []byte(c)); err != nil {
						t.Fatal(err)
					}
				},
				Read: func(r model.Root, path string) (string, error) {
					b, err := p.Tree.Read(r, path)
					return string(b), err
				},
			}
		})
	})
}

// seedWork seeds root's tree with the fixture's work tree, its files and
// its links, as the adapter's builder writes them over the index's.
func seedWork(w *porttest.World, dir string, f contracttest.HistoryFixture) {
	root := w.Seed(dir, f.Work)
	for p, to := range f.WorkLinks {
		w.Link(root, p, to)
	}
}

// repoOf is the fake's seed of the fixture f: each tag and branch head with
// its tree and links, the index, the merge base of every pair of revs, and
// the messages of every range of two commits, worked out here from each
// commit's parents, a merge's both, since the fake never derives one.
func repoOf(f contracttest.HistoryFixture) porttest.Repo {
	byName := map[string]contracttest.FixtureCommit{}
	parents := map[string][]string{}
	head := map[string]string{}
	for _, c := range f.Commits {
		byName[c.Name] = c
		if h, ok := head[c.Branch]; ok {
			parents[c.Name] = append(parents[c.Name], h)
		} else if c.From != "" {
			parents[c.Name] = append(parents[c.Name], c.From)
		}
		if c.Merge != "" {
			parents[c.Name] = append(parents[c.Name], c.Merge)
		}
		head[c.Branch] = c.Name
	}
	r := porttest.Repo{
		Tags: map[string]porttest.Rev{}, Heads: map[string]porttest.Rev{}, Index: f.Index, IndexLinks: f.IndexLinks,
		Bases: map[[2]string]string{}, Logs: map[[2]string][]model.Message{}, Encodings: map[string]string{},
	}
	revs := map[string]string{} // every rev, as the name of its commit
	for _, tag := range f.Tags {
		c := byName[tag.Commit]
		r.Tags[tag.Name] = porttest.Rev{Commit: c.ID, Files: c.Files, Links: c.Links}
		revs[tag.Name] = c.Name
	}
	for branch, name := range head {
		c := byName[name]
		r.Heads[branch] = porttest.Rev{Commit: c.ID, Files: c.Files, Links: c.Links}
		revs[branch] = name
	}
	r.Parents = map[string][]string{}
	for _, c := range f.Commits {
		revs[c.ID] = c.Name
		r.Revs = append(r.Revs, porttest.Rev{Commit: c.ID, Files: c.Files, Links: c.Links})
		var ids []string
		for _, p := range parents[c.Name] {
			ids = append(ids, byName[p].ID)
		}
		r.Parents[c.ID] = ids
		if c.Encoding != "" {
			r.Encodings[c.ID] = c.Encoding
		}
	}
	for a, ca := range revs {
		for b, cb := range revs {
			r.Bases[[2]string{a, b}] = byName[best(parents, ca, cb)].ID
		}
	}
	for _, x := range f.Commits {
		for _, y := range f.Commits {
			lacks := ancestors(parents, x.Name)
			msgs := []model.Message{}
			for name := range ancestors(parents, y.Name) {
				if !lacks[name] {
					msgs = append(msgs, model.Message{Commit: byName[name].ID, Text: byName[name].Stored()})
				}
			}
			r.Logs[[2]string{x.ID, y.ID}] = msgs
		}
	}
	return r
}

// ancestors is name and every commit it descends from, through every parent.
func ancestors(parents map[string][]string, name string) map[string]bool {
	seen := map[string]bool{}
	for next := []string{name}; len(next) > 0; {
		n := next[len(next)-1]
		next = next[:len(next)-1]
		if !seen[n] {
			seen[n] = true
			next = append(next, parents[n]...)
		}
	}
	return seen
}

// best is the merge base of a and b: the one commit both descend from that
// no other such commit descends from, or "" when there is none or more than
// one.
func best(parents map[string][]string, a, b string) string {
	inB := ancestors(parents, b)
	var common []string
	for n := range ancestors(parents, a) {
		if inB[n] {
			common = append(common, n)
		}
	}
	var bases []string
	for _, n := range common {
		later := false
		for _, m := range common {
			later = later || m != n && ancestors(parents, m)[n]
		}
		if !later {
			bases = append(bases, n)
		}
	}
	if len(bases) != 1 {
		return ""
	}
	return bases[0]
}

// TestTheFakeFillsEveryPortOfThePortPackage: Ports has every one of its
// seventeen fields filled, the optional ones too, Plugins among them
// (arch.ports.17; Tool went).
func TestTheFakeFillsEveryPortOfThePortPackage(t *testing.T) {
	t.Parallel()
	p := porttest.NewWorld(t).Ports()
	missing := porttest.MissingPorts(p)
	if len(missing) != 0 {
		t.Errorf("the fake leaves %v empty", missing)
	}
	if n := porttest.CountPorts(p); n != 17 {
		t.Errorf("Ports has %d fields, want 17", n)
	}
	t.Run("an empty port is seen", func(t *testing.T) {
		t.Parallel()
		p.Clock = nil
		if got := porttest.MissingPorts(p); len(got) != 1 || got[0] != "Clock" {
			t.Errorf("MissingPorts = %v, want [Clock]", got)
		}
	})
}

// oneScreen is a whole screen of one closing word.
func oneScreen() model.Screen {
	c, err := model.NewClosing(model.Done, "all of it", "")
	if err != nil {
		panic(err)
	}
	s, err := model.NewScreen(model.Heading{Text: "check"}, c)
	if err != nil {
		panic(err)
	}
	return s
}

// TestTheTerminalKeepsTheScreensItShowed: a screen shown is recorded whole and
// in order, and a screen with no block is refused and not recorded.
func TestTheTerminalKeepsTheScreensItShowed(t *testing.T) {
	t.Parallel()
	w := porttest.NewWorld(t)
	w.SeedTerminal(porttest.Term{})
	term := w.Ports().Terminal
	if err := term.Show(model.Screen{}); err == nil {
		t.Error("a screen with no block was shown")
	}
	for range 2 {
		if err := term.Show(oneScreen()); err != nil {
			t.Fatal(err)
		}
	}
	if len(w.Shown) != 2 || len(w.Shown[0].Blocks()) != 2 {
		t.Errorf("recorded %d screens, want the two that were shown", len(w.Shown))
	}
}

// sinceIs is whether err names the start time since.
func sinceIs(err error, since int64) bool {
	got, named := porttest.HeldSince(err)
	return named && got == since
}

// TestTheFakeHistoryNamesTheCommitWhoseMessageItCannotGiveWhole: the
// contract suite holds Messages to refusing a message past a NUL or under
// another encoding than UTF-8; the port also promises the error names the
// commit, which only a test file may read (go.errors.8). The adapter's is
// held in internal/host/git; this is the fake's.
func TestTheFakeHistoryNamesTheCommitWhoseMessageItCannotGiveWhole(t *testing.T) {
	t.Parallel()
	f := contracttest.TheHistory()
	id := map[string]string{}
	for _, c := range f.Commits {
		id[c.Name] = c.ID
	}
	w := porttest.NewWorld(t)
	w.SeedRepo("/work/book", repoOf(f))
	h := w.Ports().History
	for _, name := range []string{"hidden", "latin"} {
		got, err := h.Messages(t.Context(), "/work/book", id["odd"], id[name])
		if id[name] == "" || err == nil || got != nil || !strings.Contains(err.Error(), id[name]) {
			t.Errorf("Messages(odd, %s) = %q, %v; want an error naming %s", name, got, err, id[name])
		}
	}
}
