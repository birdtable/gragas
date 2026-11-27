package tool_test

import (
	"context"
	"encoding/base64"
	"errors"
	"github.com/birdtable/go-files/filestest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	testenv "github.com/birdtable/go-testkit"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/host/tool"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/port"
	"github.com/birdtable/gragas/internal/port/contracttest"
)

// The host's step runner is the Steps port.
var _ port.Steps = (*tool.Tool)(nil)

// TestTheStepRunnerKeepsThePortsContract: the adapter is held to the suite
// the fake is (tests.doubles.4).
func TestTheStepRunnerKeepsThePortsContract(t *testing.T) {
	t.Parallel()
	contracttest.Steps(t, func(t *testing.T, note func(context.Context, plugin.Noted) error) (port.Steps, plugin.Scope) {
		t.Helper()
		s, tl, _ := steps(t, model.Isolated, plugin.LevelIsolated, &notes{})
		s.Note = note
		return tl, s
	})
}

// notes is a scope's Note that keeps every step it is handed, and answers
// fail for each when fail is set.
type notes struct {
	mu sync.Mutex
	// got is every note, a step's and a version ask's, in order.
	got []plugin.Noted
	// fail is what a step's note answers, and failAsk a version ask's.
	fail, failAsk error
}

func (n *notes) note(_ context.Context, s plugin.Noted) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.got = append(n.got, s)
	if s.VersionAsk {
		return n.failAsk
	}
	return n.fail
}

// all is the steps' notes, in order, and asks the version asks'.
func (n *notes) all() []plugin.Noted { return n.of(false) }

func (n *notes) asks() []plugin.Noted { return n.of(true) }

func (n *notes) of(ask bool) []plugin.Noted {
	n.mu.Lock()
	defer n.mu.Unlock()
	var out []plugin.Noted
	for _, x := range n.got {
		if x.VersionAsk == ask {
			out = append(out, x)
		}
	}
	return out
}

// stepManifest is foundGrant serving the tool lint-a: the grant the runner
// finds and asks its version at Bind.
func stepManifest() plugin.Manifest {
	m := foundGrant()
	m.Tool = "lint-a"
	return m
}

// steps is a project, a program lint-a named for it in the settings that
// says lint-a 1.2.3 for --version and otherwise does body's steps, a scratch
// folder that reached reached, and the scope over them with the floor and the
// note n. It answers the scope, the adapter and the program's path.
func steps(t *testing.T, reached model.Level, floor plugin.Level, n *notes, body ...configtest.Step) (plugin.Scope, *tool.Tool, string) {
	t.Helper()
	root := private(t)
	prog := program(t, private(t)+"/lint-a", answering(body...))
	tl := over(map[string]map[string]string{root: {"lint-a@1.2.3": prog}}, nil)
	s := plugin.Scope{Manifest: stepManifest(), Root: model.Root(root), Floor: floor, Note: n.note,
		Scratch: model.Scratch{Dir: private(t), Level: reached, Seconds: 60}}
	return s, tl, prog
}

// bind is tl bound to s, or the test's end.
func bind(t *testing.T, tl *tool.Tool, s plugin.Scope) port.StepRunner {
	t.Helper()
	r, err := tl.Bind(t.Context(), s)
	if err != nil {
		t.Fatalf("Bind = %v", err)
	}
	return r
}

// runOf is a step that runs lint-a with argv.
func runOf(argv ...string) plugin.Step {
	return plugin.Step{Run: &plugin.Run{Program: "lint-a", Argv: argv}}
}

// text is the bytes b holds, text or base64.
func text(t *testing.T, b *plugin.Bytes) string {
	t.Helper()
	switch {
	case b == nil:
		return ""
	case b.Text != nil:
		return *b.Text
	case b.Base64 != nil:
		data, err := base64.StdEncoding.DecodeString(*b.Base64)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	return ""
}

// stepRuns is how many runs tl started past Bind's one version run.
func stepRuns(tl *tool.Tool) int { return len(tl.Ran()) - 1 }

// TestABoundRunnerFindsItsToolFirst (item 8): Bind finds the grant whose id
// is the manifest's tool and reads its version before it answers, so the
// version is known before any plugin call; a scope it cannot run under is
// refused with nothing run; a tool that is nowhere here is not ready.
// Planted: Bind answering a NotFound as it is, and Bind taking a nil Note.
func TestABoundRunnerFindsItsToolFirst(t *testing.T) {
	t.Parallel()
	t.Run("the tool and its version", func(t *testing.T) {
		t.Parallel()
		s, tl, prog := steps(t, model.Isolated, plugin.LevelIsolated, &notes{})
		r := bind(t, tl, s)
		if f := r.Tool(); f.Path != prog || f.Version != "1.2.3" {
			t.Fatalf("Tool() = %+v, want %s at 1.2.3", f, prog)
		}
		if ran := tl.Ran(); len(ran) != 1 || ran[0].Argv[1] != "--version" {
			t.Fatalf("Bind ran %+v, want the one version run", ran)
		}
	})
	for name, c := range map[string]struct {
		change func(*plugin.Scope)
		says   string
	}{
		"no note":                 {func(s *plugin.Scope) { s.Note = nil }, "no note"},
		"a relative scratch":      {func(s *plugin.Scope) { s.Scratch.Dir = "scratch" }, "the scratch folder"},
		"a relative root":         {func(s *plugin.Scope) { s.Root = "project" }, "the root"},
		"a scratch of no level":   {func(s *plugin.Scope) { s.Scratch.Level = "" }, "a level is"},
		"a floor that is neither": {func(s *plugin.Scope) { s.Floor = "loose" }, "a level is"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s, tl, _ := steps(t, model.Isolated, plugin.LevelIsolated, &notes{})
			c.change(&s)
			_, err := tl.Bind(t.Context(), s)
			endsAs(t, err, c.says, port.ErrRefused)
			if len(tl.Ran()) != 0 {
				t.Errorf("a refused scope ran %+v", tl.Ran())
			}
		})
	}
	t.Run("a tool that is nowhere", func(t *testing.T) {
		t.Parallel()
		s, _, _ := steps(t, model.Isolated, plugin.LevelIsolated, &notes{})
		_, err := over(nil, nil).Bind(t.Context(), s)
		endsAs(t, err, "so the step is not ready", port.ErrStepNotReady)
	})
	t.Run("a grant the manifest lacks", func(t *testing.T) {
		t.Parallel()
		s, tl, _ := steps(t, model.Isolated, plugin.LevelIsolated, &notes{})
		s.Manifest.Tool = "other"
		if _, err := tl.Bind(t.Context(), s); !errors.Is(err, port.ErrGrant) {
			t.Fatalf("Bind of a tool with no grant = %v, want port.ErrGrant", err)
		}
	})
}

// TestGrantedLooksAtNothing: a read is granted, a run of a grant is, a run
// of any other program matches port.ErrGrant and is never started.
func TestGrantedLooksAtNothing(t *testing.T) {
	t.Parallel()
	s, tl, _ := steps(t, model.Isolated, plugin.LevelIsolated, &notes{})
	r := bind(t, tl, s)
	if err := r.Granted(plugin.Step{Read: &plugin.Read{Glob: "*.json"}}); err != nil {
		t.Errorf("Granted(read) = %v", err)
	}
	if err := r.Granted(runOf("x")); err != nil {
		t.Errorf("Granted(lint-a) = %v", err)
	}
	other := plugin.Step{Run: &plugin.Run{Program: "other"}}
	if err := r.Granted(other); !errors.Is(err, port.ErrGrant) {
		t.Errorf("Granted(other) = %v, want port.ErrGrant", err)
	}
	if _, err := r.Run(t.Context(), 0, 0, other); !errors.Is(err, port.ErrGrant) || stepRuns(tl) != 0 {
		t.Errorf("Run(other) = %v after %d runs, want port.ErrGrant and none", err, stepRuns(tl))
	}
	for name, st := range map[string]plugin.Step{"neither": {}, "both": {Run: runOf().Run, Read: &plugin.Read{Glob: "*"}}} {
		if _, err := r.Run(t.Context(), 0, 0, st); !errors.Is(err, port.ErrRefused) {
			t.Errorf("Run of %s = %v, want refused", name, err)
		}
	}
}

// TestAStepRunsAtTheHigherLevel (item 6): a step runs at the higher of the
// engine's floor and its own; asking less than the floor runs at the floor;
// a level the scratch folder did not reach is one this machine lacks, so
// the step is not ready, nothing journalled and nothing started. Planted:
// the lacking level answered as a plain refusal, and the asked level taken
// over the floor.
func TestAStepRunsAtTheHigherLevel(t *testing.T) {
	t.Parallel()
	iso, sand := plugin.LevelIsolated, plugin.LevelSandboxed
	for name, c := range map[string]struct {
		reached model.Level
		floor   plugin.Level
		asked   *plugin.Level
		want    plugin.Level
	}{
		"the floor, asking nothing":     {model.Isolated, iso, nil, iso},
		"its own, above the floor":      {model.Sandboxed, iso, &sand, sand},
		"the floor, asking less":        {model.Sandboxed, sand, &iso, sand},
		"a level the machine lacks":     {model.Isolated, iso, &sand, ""},
		"a floor the machine lacks":     {model.Isolated, sand, nil, ""},
		"a floor the machine lacks too": {model.Isolated, sand, &iso, ""},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			n := &notes{}
			s, tl, _ := steps(t, c.reached, c.floor, n, configtest.Exit(0))
			st := runOf()
			st.Run.Level = c.asked
			_, err := bind(t, tl, s).Run(t.Context(), 0, 0, st)
			if c.want == "" {
				endsAs(t, err, "reached "+string(c.reached), port.ErrStepNotReady)
				if len(n.all()) != 0 || stepRuns(tl) != 0 {
					t.Errorf("a step not ready was journalled %+v or run %d times", n.all(), stepRuns(tl))
				}
				return
			}
			if got := n.all(); err != nil || len(got) != 1 || got[0].Level != c.want {
				t.Fatalf("Run = %v, noted %+v; want it run at %s", err, got, c.want)
			}
		})
	}
}

// TestAStepIsJournalledBeforeItRuns (item 7): the note holds the grant, the
// very path started, the version, the words as the program is given them and
// the level; a note that fails starts nothing and places nothing. Planted:
// the note called after the start, and its error dropped.
func TestAStepIsJournalledBeforeItRuns(t *testing.T) {
	t.Parallel()
	t.Run("the note", func(t *testing.T) {
		t.Parallel()
		n := &notes{}
		s, tl, prog := steps(t, model.Isolated, plugin.LevelIsolated, n, configtest.PrintArgs())
		if _, err := bind(t, tl, s).Run(t.Context(), 1, 2, runOf("check", "{scratch}/a")); err != nil {
			t.Fatal(err)
		}
		words, err := tool.StepWords([]string{"check", "{scratch}/a"}, filepath.FromSlash(s.Scratch.Dir))
		if err != nil {
			t.Fatal(err)
		}
		got, ran := n.all(), tl.Ran()
		want := plugin.Noted{Round: 1, Step: 2, Grant: "lint-a", Program: prog, Version: "1.2.3", Words: words, Level: plugin.LevelIsolated}
		if len(got) != 1 || !same(got[0], want) || ran[len(ran)-1].Argv[0] != got[0].Program {
			t.Fatalf("noted %+v, started %+v; want %+v, the path noted the one started", got, ran[len(ran)-1], want)
		}
	})
	t.Run("a note that fails", func(t *testing.T) {
		t.Parallel()
		n := &notes{fail: errors.New("the journal is full")}
		s, tl, _ := steps(t, model.Isolated, plugin.LevelIsolated, n, configtest.Exit(0))
		st := runOf()
		st.Run.Place = []plugin.File{{Path: "a.txt", Data: plugin.Bytes{Text: new("one")}}}
		_, err := bind(t, tl, s).Run(t.Context(), 0, 0, st)
		if !errors.Is(err, n.fail) || stepRuns(tl) != 0 {
			t.Fatalf("Run = %v after %d runs, want the note's error and nothing run", err, stepRuns(tl))
		}
		if _, err := os.Lstat(s.Scratch.Dir + "/a.txt"); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("a step whose note failed placed its file: %v", err)
		}
	})
}

// same is whether two notes are equal.
func same(a, b plugin.Noted) bool {
	return a.Round == b.Round && a.Step == b.Step && a.Grant == b.Grant && a.Program == b.Program && a.VersionAsk == b.VersionAsk &&
		a.Script == b.Script && a.Version == b.Version && a.Level == b.Level && strings.Join(a.Words, "\x00") == strings.Join(b.Words, "\x00")
}

// TestAStepRunsItsWordsAndItsStdin (items 4 and 5): the words with their
// placeholders filled, the exit code and stderr as the program gave them,
// stdin as text or base64; stdin past plugin.MaxStdin, or both forms or
// neither, is refused with nothing started. Planted: the stdin cap skipped.
func TestAStepRunsItsWordsAndItsStdin(t *testing.T) {
	t.Parallel()
	t.Run("words, exit and stderr", func(t *testing.T) {
		t.Parallel()
		s, tl, _ := steps(t, model.Isolated, plugin.LevelIsolated, &notes{}, configtest.PrintArgs(), configtest.Err("oops"), configtest.Exit(3))
		res, err := bind(t, tl, s).Run(t.Context(), 0, 0, runOf("check", "{scratch}"))
		want := "check\n" + filepath.FromSlash(s.Scratch.Dir) + "\n"
		if err != nil || res.Exit == nil || *res.Exit != 3 || text(t, res.Stdout) != want || text(t, res.Stderr) != "oops" || res.Cut == nil || *res.Cut {
			t.Fatalf("Run = %+v, %v; want exit 3, stdout %q, stderr oops, not cut", res, err, want)
		}
	})
	for name, in := range map[string]plugin.Bytes{
		"text":   {Text: new("hello\n")},
		"base64": {Base64: new(base64.StdEncoding.EncodeToString([]byte("hello\n")))},
	} {
		t.Run("stdin as "+name, func(t *testing.T) {
			t.Parallel()
			s, tl, _ := steps(t, model.Isolated, plugin.LevelIsolated, &notes{}, configtest.Stdin())
			st := runOf()
			st.Run.Stdin = &in
			res, err := bind(t, tl, s).Run(t.Context(), 0, 0, st)
			if err != nil || text(t, res.Stdout) != "hello\n" {
				t.Fatalf("Run = %+v, %v; want stdin read back", res, err)
			}
		})
	}
	for name, c := range map[string]struct {
		in   plugin.Bytes
		says string
	}{
		"past the cap": {plugin.Bytes{Text: new(strings.Repeat("x", plugin.MaxStdin+1))}, "more than the"},
		"both forms":   {plugin.Bytes{Text: new("a"), Base64: new("YQ==")}, "one of the two"},
		"neither":      {plugin.Bytes{}, "one of the two"},
		"no base64":    {plugin.Bytes{Base64: new("not base64!")}, "does not decode"},
	} {
		t.Run("stdin "+name, func(t *testing.T) {
			t.Parallel()
			n := &notes{}
			s, tl, _ := steps(t, model.Isolated, plugin.LevelIsolated, n, configtest.Stdin())
			st := runOf()
			st.Run.Stdin = &c.in
			_, err := bind(t, tl, s).Run(t.Context(), 0, 0, st)
			endsAs(t, err, c.says, port.ErrRefused)
			if stepRuns(tl) != 0 || len(n.all()) != 0 {
				t.Errorf("a refused stdin was journalled or run")
			}
		})
	}
	t.Run("stdin at the cap", func(t *testing.T) {
		t.Parallel()
		s, tl, _ := steps(t, model.Isolated, plugin.LevelIsolated, &notes{}, configtest.Exit(0))
		st := runOf()
		st.Run.Stdin = &plugin.Bytes{Text: new(strings.Repeat("x", plugin.MaxStdin))}
		if _, err := bind(t, tl, s).Run(t.Context(), 0, 0, st); err != nil {
			t.Fatalf("Run of stdin at the cap = %v", err)
		}
	})
}

// TestAStreamIsCutAtItsCap (item 5): a run that prints past plugin.MaxStream
// is ended, its stream kept to the cap and cut said, with no exit code.
// Planted: the step run at the adapter's cap.
func TestAStreamIsCutAtItsCap(t *testing.T) {
	t.Parallel()
	s, tl, _ := steps(t, model.Isolated, plugin.LevelIsolated, &notes{}, configtest.Flood("x"))
	res, err := bind(t, tl, s).Run(t.Context(), 0, 0, runOf())
	if err != nil || res.Cut == nil || !*res.Cut || res.Exit != nil || len(text(t, res.Stdout)) != plugin.MaxStream {
		t.Fatalf("Run = cut %v, exit %v, %d bytes, %v; want cut at %d with no exit", res.Cut, res.Exit, len(text(t, res.Stdout)), err, plugin.MaxStream)
	}
}

// TestAStepPlacesAndCollectsBelowItsScratch (item 5): a placed file is
// written below the scratch folder and collected back; a collect of a file
// that is not there is left out; a path that climbs, a placed file over one
// that is there, and a folder through a link are refused, and a refused path
// starts nothing.
func TestAStepPlacesAndCollectsBelowItsScratch(t *testing.T) {
	t.Parallel()
	t.Run("placed and collected", func(t *testing.T) {
		t.Parallel()
		s, tl, _ := steps(t, model.Isolated, plugin.LevelIsolated, &notes{}, configtest.Exit(0))
		st := runOf()
		st.Run.Place = []plugin.File{{Path: "in/a.txt", Data: plugin.Bytes{Text: new("one")}}}
		st.Run.Collect = []string{"in/a.txt", "none.txt"}
		res, err := bind(t, tl, s).Run(t.Context(), 0, 0, st)
		if err != nil || len(res.Files) != 1 || res.Files[0].Path != "in/a.txt" || text(t, &res.Files[0].Data) != "one" {
			t.Fatalf("Run = %+v, %v; want in/a.txt collected as one", res.Files, err)
		}
	})
	for name, c := range map[string]struct {
		place, collect string
		before         func(t *testing.T, scratch string)
		says           string
	}{
		"a place that climbs":   {place: "../a.txt", says: "a.txt"},
		"a collect that climbs": {collect: "../a.txt", says: "a.txt"},
		"over a file": {place: "a.txt", says: "a file is there already", before: func(t *testing.T, scratch string) {
			t.Helper()
			filestest.WriteFile(t, scratch+"/a.txt", []byte("mirrored"))
		}},
		"through a link": {place: "l/a.txt", says: "through a link", before: func(t *testing.T, scratch string) {
			t.Helper()
			testenv.Link(t, private(t), scratch+"/l")
		}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s, tl, _ := steps(t, model.Isolated, plugin.LevelIsolated, &notes{}, configtest.Exit(0))
			if c.before != nil {
				c.before(t, s.Scratch.Dir)
			}
			st := runOf()
			if c.place != "" {
				st.Run.Place = []plugin.File{{Path: c.place, Data: plugin.Bytes{Text: new("one")}}}
			}
			if c.collect != "" {
				st.Run.Collect = []string{c.collect}
			}
			_, err := bind(t, tl, s).Run(t.Context(), 0, 0, st)
			endsAs(t, err, c.says, port.ErrRefused)
			if c.place != "" && stepRuns(tl) != 0 {
				t.Errorf("a refused place ran %d times", stepRuns(tl))
			}
		})
	}
}

// TestAReadStepReadsItsScratch: a read gives the files its glob matches in
// the scratch folder, and runs nothing.
func TestAReadStepReadsItsScratch(t *testing.T) {
	t.Parallel()
	s, tl, _ := steps(t, model.Isolated, plugin.LevelIsolated, &notes{})
	filestest.WriteFile(t, s.Scratch.Dir+"/a.json", []byte("{}"))
	filestest.WriteFile(t, s.Scratch.Dir+"/b.txt", []byte("no"))
	res, err := bind(t, tl, s).Run(t.Context(), 0, 0, plugin.Step{Read: &plugin.Read{Glob: "*.json"}})
	if err != nil || len(res.Files) != 1 || res.Files[0].Path != "a.json" || stepRuns(tl) != 0 {
		t.Fatalf("Run(read) = %+v, %v after %d runs; want a.json alone and none", res.Files, err, stepRuns(tl))
	}
}

// TestAScriptStepPutsItsScriptFirst (item 3): a script grant's step runs the
// interpreter with the script as its first word, and the plan's words after
// it, so a word such as -e is the script's and never the interpreter's.
// Planted: the words put before the script.
func TestAScriptStepPutsItsScriptFirst(t *testing.T) {
	t.Parallel()
	const pj = "node_modules/lint-a/package.json"
	root, tl := pkg(t, `{"tools": {"lint-a": "bin/lint.js"}}`)
	m := scriptGrant(pj, "/tools/lint-a")
	m.Tool = "lint-a"
	n := &notes{}
	s := plugin.Scope{Manifest: m, Root: model.Root(root), OwnPlace: true, Floor: plugin.LevelIsolated, Note: n.note,
		Scratch: model.Scratch{Dir: private(t), Level: model.Isolated, Seconds: 60}}
	res, err := bind(t, tl, s).Run(t.Context(), 0, 0, runOf("-e", "process.exit(9)"))
	script := root + "/node_modules/lint-a/bin/lint.js"
	want := "script=" + filepath.FromSlash(script) + "\n-e\nprocess.exit(9)\n"
	if err != nil || text(t, res.Stdout) != want {
		t.Fatalf("Run = %q, %v; want %q", text(t, res.Stdout), err, want)
	}
	if got := n.all(); len(got) != 1 || got[0].Script != script || got[0].Program != root+"/runtime/"+filepath.Base(got[0].Program) {
		t.Errorf("noted %+v, want node and the script %s", got, script)
	}
}
