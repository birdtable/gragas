package apply_test

import (
	"context"
	"io/fs"
	"path"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/birdtable/gragas/internal/apply"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// activeA is the digest of rules/a.yaml as the fixture seeds it.
func activeA() model.Digest { return apply.DigestBytes([]byte("status: active\n")) }

// rawSpec is a Spec whose plan names exactly the inputs and targets given, so
// a test says what the plan holds and not what a test plan would make of it.
func rawSpec(f *fixture, inputs []model.Input, targets ...model.Target) apply.Spec {
	s := f.plan().spec()
	s.Plan = func(context.Context) (model.Plan, error) {
		return model.Plan{Delta: model.Delta{Changes: []model.Change{{Entity: "file:x", Field: "content"}}}, Inputs: inputs}, nil
	}
	s.Render = func(model.Delta) ([]model.Target, error) { return targets, nil }
	return s
}

// errTree answers err for a read of path, and the inner tree for the rest.
type errTree struct {
	port.Tree
	path string
	err  error
}

func (e errTree) Read(r model.Root, p string) ([]byte, error) {
	if p == e.path {
		return nil, e.err
	}
	return e.Tree.Read(r, p)
}

func wantNoEffect(t *testing.T, f *fixture) {
	t.Helper()
	if len(f.w.Writes) != 0 {
		t.Errorf("wrote %v", f.w.Writes)
	}
	if got := f.content("rules/a.yaml"); got != "status: active\n" {
		t.Errorf("rules/a.yaml now holds %q", got)
	}
}

// TestACheckThatCouldNotRunIsNeverAPass: a read the check cannot make ends the
// run, and a missing file is only what the tree says is missing. Risk: a read
// error taken as a match (B12), any error taken as "not there" so a plan that
// expects no file writes over one it could not read (B13), a path that leaves
// the root no longer a refusal (B11), and a target's own Before never compared
// when the file is not an input (B10).
func TestACheckThatCouldNotRunIsNeverAPass(t *testing.T) {
	t.Parallel()
	target := func(before model.Digest) model.Target {
		return model.Target{Path: "rules/a.yaml", Before: before, Data: []byte("status: retired\n")}
	}
	t.Run("an input that cannot be read (B12)", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		f.p.Tree = errTree{f.p.Tree, "identity.yaml", errBoom}
		o := f.run(rawSpec(f, []model.Input{{Path: "identity.yaml", Digest: apply.DigestBytes([]byte("name: demo\n"))}}, target(activeA())))
		if o.Word != model.Failed || o.Failure == nil || o.Failure.Kind() != apply.FailRead {
			t.Errorf("ended %q %+v, want failed, kind read", o.Word, o.Failure)
		}
		wantNoEffect(t, f)
	})
	t.Run("a target that cannot be read (B12)", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		f.p.Tree = errTree{f.p.Tree, "rules/a.yaml", errBoom}
		o := f.run(rawSpec(f, nil, target(activeA())))
		if o.Word != model.Failed || o.Failure == nil || o.Failure.Kind() != apply.FailRead {
			t.Errorf("ended %q %+v, want failed, kind read", o.Word, o.Failure)
		}
		wantNoEffect(t, f)
	})
	t.Run("an unreadable file the plan expects absent (B13)", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		f.p.Tree = errTree{f.p.Tree, "rules/a.yaml", errBoom}
		o := f.run(rawSpec(f, nil, target("")))
		if o.Word != model.Failed || o.Failure == nil || o.Failure.Kind() != apply.FailRead {
			t.Errorf("ended %q %+v, want failed, kind read", o.Word, o.Failure)
		}
		wantNoEffect(t, f)
	})
	t.Run("a file that is there when the plan expects it absent (B13)", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		o := f.run(rawSpec(f, nil, target("")))
		if o.Word != model.Refused || o.Refusal == nil || !strings.Contains(o.Refusal.Observed(), "rules/a.yaml") {
			t.Errorf("ended %q %+v, want refused naming rules/a.yaml", o.Word, o.Refusal)
		}
		wantNoEffect(t, f)
	})
	t.Run("a file that is missing when the plan expects it absent still writes", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		o := f.run(rawSpec(f, nil, model.Target{Path: "rules/new.yaml", Data: []byte("status: trial\n")}))
		wantDone(t, o)
		if got := f.content("rules/new.yaml"); got != "status: trial\n" {
			t.Errorf("rules/new.yaml holds %q", got)
		}
	})
	for _, name := range []string{"an input", "a target"} {
		t.Run(name+" that leaves the root (B11)", func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			f.p.Tree = errTree{f.p.Tree, "rules/a.yaml", port.ErrOutside}
			var in []model.Input
			var targets []model.Target
			if name == "an input" {
				in = []model.Input{{Path: "rules/a.yaml", Digest: activeA()}}
				targets = []model.Target{{Path: "rules/b.yaml", Before: apply.DigestBytes([]byte("status: active\n")), Data: []byte("x")}}
			} else {
				targets = []model.Target{target(activeA())}
			}
			o := f.run(rawSpec(f, in, targets...))
			if o.Word != model.Refused || o.Refusal == nil || !strings.Contains(o.Refusal.Observed(), "rules/a.yaml leaves the root: "+port.ErrOutside.Error()) {
				t.Errorf("ended %q %+v, want refused with the error text", o.Word, o.Refusal)
			}
			if got, want := kinds(f.records()), []string{"intent", "close"}; !slices.Equal(got, want) {
				t.Errorf("records %q, want %q", got, want)
			}
			wantNoEffect(t, f)
		})
	}
	t.Run("a target that is not an input and changed since the plan (B10)", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		o := f.run(rawSpec(f, nil, target(apply.DigestBytes([]byte("status: trial\n")))))
		if o.Word != model.Refused || o.Refusal == nil || !strings.Contains(o.Refusal.Observed(), "rules/a.yaml") {
			t.Errorf("ended %q %+v, want refused naming rules/a.yaml", o.Word, o.Refusal)
		}
		wantNoEffect(t, f)
	})
}

// writeFlag is shared by a Writer and a Tree that wrap the same ports: the
// tree fails its reads once the writer has written.
type writeFlag struct{ wrote bool }

type flagWriter struct {
	port.Writer
	flag *writeFlag
}

func (w flagWriter) Replace(r model.Root, p string, data []byte) error {
	w.flag.wrote = true
	return w.Writer.Replace(r, p, data)
}

func (w flagWriter) Append(r model.Root, p string, line []byte) error {
	w.flag.wrote = true
	return w.Writer.Append(r, p, line)
}

type flagTree struct {
	port.Tree
	flag *writeFlag
}

func (t flagTree) Read(r model.Root, p string) ([]byte, error) {
	if t.flag.wrote {
		return nil, errBoom
	}
	return t.Tree.Read(r, p)
}

// TestAReadBackThatFailsEndsFailedReadWithTheRecordOpen (B34): the file was
// written and cannot be shown to hold it; the run says so and does not close.
func TestAReadBackThatFailsEndsFailedReadWithTheRecordOpen(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	flag := &writeFlag{}
	f.p.Writer = flagWriter{f.p.Writer, flag}
	f.p.Tree = flagTree{f.p.Tree, flag}
	o := f.run(f.plan(edit{path: "rules/a.yaml", data: "status: retired\n"}).spec())
	if o.Word != model.Failed || o.Failure == nil || o.Failure.Kind() != apply.FailRead {
		t.Errorf("ended %q %+v, want failed, kind read", o.Word, o.Failure)
	}
	if got, want := kinds(f.records()), []string{"intent", "copy", "step"}; !slices.Equal(got, want) {
		t.Errorf("records %q, want %q: no close", got, want)
	}
}

// TestAnInputPathThatIsNotSafeIsRefusedBeforeTheIntent (B21): the same vet
// that holds a target holds an input, since the check reads it.
func TestAnInputPathThatIsNotSafeIsRefusedBeforeTheIntent(t *testing.T) {
	t.Parallel()
	for _, p := range []string{"../identity.yaml", "/work/scratch/hosts", ".git/config", backslashed(t)["name"], "", "a:b"} {
		f := newFixture(t)
		o := f.run(rawSpec(f, []model.Input{{Path: p, Digest: activeA()}}, model.Target{Path: "rules/a.yaml", Before: activeA(), Data: []byte("x")}))
		if o.Word != model.Refused {
			t.Errorf("input %q: ended %q, want refused", p, o.Word)
		}
		if len(f.records()) != 0 {
			t.Errorf("input %q: journalled %d records", p, len(f.records()))
		}
		wantNoEffect(t, f)
	}
}

// ctxBlindLock takes the lock whatever the context says, so a cancelled run
// reaches the check the write path makes for itself.
type ctxBlindLock struct{ port.Lock }

func (l ctxBlindLock) Take(ctx context.Context, r model.Root, w model.Lock) (model.Hold, error) {
	return l.Lock.Take(context.WithoutCancel(ctx), r, w)
}

func (l ctxBlindLock) Wait(ctx context.Context, r model.Root, w model.Lock) (model.Hold, error) {
	return l.Lock.Wait(context.WithoutCancel(ctx), r, w)
}

// TestACancelledContextEndsBeforeThePlanIsMade (B25): nothing is planned for a
// run its context already stopped.
func TestACancelledContextEndsBeforeThePlanIsMade(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.p.Lock = ctxBlindLock{f.p.Lock}
	s := f.plan(edit{path: "rules/a.yaml", data: "status: retired\n"}).spec()
	planned := false
	plan := s.Plan
	s.Plan = func(c context.Context) (model.Plan, error) { planned = true; return plan(c) }
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	o := apply.Run(ctx, f.p, f.root, s)

	if o.Word != model.Failed || o.Failure == nil {
		t.Fatalf("ended %q %+v, want failed", o.Word, o.Failure)
	}
	if o.Failure.Kind() != apply.FailCancelled {
		t.Errorf("kind %s, want cancelled", o.Failure.Kind())
	}
	if planned {
		t.Error("the plan function ran for a cancelled run")
	}
	wantNoEffect(t, f)
}

// TestThePlansCommandIsTheSpecsNotTheFunctions (B44): the journal and the
// digest name the command the caller gave, whatever the plan function says.
func TestThePlansCommandIsTheSpecsNotTheFunctions(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	s := f.plan(edit{path: "rules/a.yaml", data: "status: retired\n"}).spec()
	plan := s.Plan
	s.Plan = func(c context.Context) (model.Plan, error) {
		p, err := plan(c)
		p.Command = "gragas something-else --apply"
		return p, err
	}
	o := f.run(s)
	wantDone(t, o)
	if o.Plan.Command != s.Command {
		t.Errorf("plan command %q, want %q", o.Plan.Command, s.Command)
	}
	for _, r := range f.records() {
		if r.Command != s.Command {
			t.Errorf("%s record names %q, want %q", r.Kind, r.Command, s.Command)
		}
	}
}

// TestFilesWithAnEmptyDeltaAreStillWritten (B49): "nothing to do" is no
// changes and no files; a render that answers a file for an empty delta wrote
// is not nothing.
func TestFilesWithAnEmptyDeltaAreStillWritten(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	s := f.plan().spec()
	s.Plan = func(context.Context) (model.Plan, error) { return model.Plan{}, nil }
	s.Render = func(model.Delta) ([]model.Target, error) {
		return []model.Target{{Path: "rules/a.yaml", Before: activeA(), Data: []byte("status: retired\n")}}, nil
	}
	o := f.run(s)
	wantDone(t, o)
	if o.Nothing || f.content("rules/a.yaml") != "status: retired\n" {
		t.Errorf("nothing = %v, file = %q", o.Nothing, f.content("rules/a.yaml"))
	}
}

// TestTheConfirmHintQuotesTheNameForAPOSIXShell (item 4): the line a person
// pastes cannot run a name's own words.
func TestTheConfirmHintQuotesTheNameForAPOSIXShell(t *testing.T) {
	t.Parallel()
	for name, want := range map[string]string{
		"v1.0.0":       "--confirm v1.0.0",
		"my release":   "--confirm 'my release'",
		"a;rm -rf x":   "--confirm 'a;rm -rf x'",
		"$(id)":        "--confirm '$(id)'",
		"x $(id) ; y":  "--confirm 'x $(id) ; y'",
		"it's":         `--confirm 'it'\''s'`,
		"`id`":         "--confirm '`id`'",
		"a\nb":         "--confirm 'a\nb'",
		"*":            "--confirm '*'",
		"ünï":          "--confirm 'ünï'",
		"rel/ease-1_2": "--confirm rel/ease-1_2",
	} {
		f := newFixture(t)
		s := f.plan(edit{path: "rules/a.yaml", data: "status: retired\n"}).spec()
		s.Name, s.Confirmed = name, ""
		o := f.run(s)
		if o.Word != model.Refused || o.Refusal == nil || !strings.HasSuffix(o.Refusal.Hint(), want) {
			t.Errorf("name %q: hint %v, want one ending %q", name, o.Refusal, want)
		}
	}
}

// TestAPathAMacDiskFoldsToDotGitIsRefused (item 3): the code points HFS+
// ignores make ".git" out of other names, so none is allowed in a path.
func TestAPathAMacDiskFoldsToDotGitIsRefused(t *testing.T) {
	t.Parallel()
	for _, p := range []string{
		".g\u200cit/config", ".git\u200d/config", ".\u200egit/x", ".g\u202ait/x", ".g\u206fit/x", "\ufeff.git/x",
		"rules/a\u200c.yaml",
		"GIT~1/config", "git~1/config", "x/Git~2", "git~1 /x", "GIT~12/x",
	} {
		f := newFixture(t)
		s := f.plan().spec()
		s.Scope = []string{"*", "*/*"}
		s.Plan = func(context.Context) (model.Plan, error) {
			return model.Plan{Delta: model.Delta{Changes: []model.Change{{Entity: "file:x"}}}}, nil
		}
		s.Render = func(model.Delta) ([]model.Target, error) { return []model.Target{{Path: p, Data: []byte("x")}}, nil }
		if o := f.run(s); o.Word != model.Refused || len(f.w.Writes) != 0 || len(f.records()) != 0 {
			t.Errorf("path %q: ended %q after %v", p, o.Word, f.w.Writes)
		}
	}
}

// isShortGit is the test file's own reading of a name the disk takes for .git.
func isShortGit(seg string) bool {
	l := strings.ToLower(seg)
	if l == ".git" {
		return true
	}
	n, ok := strings.CutPrefix(l, "git~")
	return ok && n != "" && strings.Trim(n, "0123456789") == ""
}

// FuzzVet holds the vet of a target path and a scope to what the host must
// never see: a path that was let through (the run journalled its intent) is
// one host/fs confine takes (fs.ValidPath, no backslash, colon or NUL), has no
// segment that folds to .git, holds no code point HFS+ ignores, and matches
// the scope. The confine rules are written out here because a pure package
// does not import host/fs; internal/host/fs holds the other half.
func FuzzVet(f *testing.F) {
	seeds := []string{
		".git/x", ".GIT/x", ".git./x", ".git /x", "a/../.git/x", "./a", "a//b", "a\x00b", "[", "*",
		".g\u200cit/config", "rules/a.yaml", "state/x", "a:b", "GIT~1", "git~1/config", "x/.git", "x/.git/config",
	}
	seeds = append(seeds, backslashed(f)["name"])
	for _, p := range seeds {
		f.Add(p, "*")
		f.Add(p, "*/*")
		f.Add(p, "[")
	}
	f.Fuzz(func(t *testing.T, p, scope string) {
		if !utf8.ValidString(p) {
			return
		}
		fx := newFixture(t)
		s := fx.plan().spec()
		s.Scope = []string{scope}
		s.Plan = func(context.Context) (model.Plan, error) {
			return model.Plan{Delta: model.Delta{Changes: []model.Change{{Entity: "file:x"}}}}, nil
		}
		s.Render = func(model.Delta) ([]model.Target, error) { return []model.Target{{Path: p, Data: []byte("x")}}, nil }
		fx.run(s)
		if len(fx.records()) == 0 && len(fx.w.Writes) == 0 {
			return // refused before the intent: nothing to hold
		}
		if !fs.ValidPath(p) || p == "." || strings.ContainsAny(p, backslashed(t)["backslash"]+":\x00") {
			t.Errorf("%q passed the vet and host/fs confine refuses it", p)
		}
		for seg := range strings.SplitSeq(p, "/") {
			if isShortGit(strings.TrimRight(seg, ". ")) {
				t.Errorf("%q passed the vet with a segment that folds to .git", p)
			}
		}
		if strings.IndexFunc(p, func(r rune) bool {
			return r >= 0x200c && r <= 0x200f || r >= 0x202a && r <= 0x202e || r >= 0x206a && r <= 0x206f || r == 0xfeff
		}) >= 0 {
			t.Errorf("%q passed the vet with a code point HFS+ ignores", p)
		}
		if ok, err := path.Match(scope, p); err != nil || !ok {
			t.Errorf("%q passed the vet outside the scope %q", p, scope)
		}
	})
}
