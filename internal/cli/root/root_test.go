package root

import (
	"context"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/birdtable/go-console/shell"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/birdtable/gragas/internal/config"
	configtest "github.com/birdtable/gragas/internal/config/testing"
	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/exit"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port/legacy"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// TestMain starts from the shared test setup.
func TestMain(m *testing.M) {
	os.Exit(configtest.NoLeaks(configtest.Main(m)))
}

// world is a rulebook in memory, with no generated file, so gen finds
// every one stale, every document valid and no names file. Inputs read:
// testdata/.
func world(t *testing.T) *porttest.Fake {
	t.Helper()
	m := fstest.MapFS{}
	for _, p := range []string{"rulebook/parts.yaml", "rulebook/products.yaml", "rulebook/sources.yaml", "rulebook/baseline.json",
		"rulebook/base/arch/rules/layers.yaml", "rulebook/base/arch/tools.yaml", "rulebook/lang/go/rules/errors.yaml", "presets/layers.yml"} {
		data, err := os.ReadFile(path.Join("testdata", p))
		if err != nil {
			t.Fatal(err)
		}
		m[p] = &fstest.MapFile{Data: data}
	}
	return &porttest.Fake{Repo: m, NamesErr: errors.New("no names file here")}
}

// process is a run of gragas in the world f, with args its arguments: none is
// the bare command, an empty list and never a nil one.
func process(t *testing.T, f *porttest.Fake, args ...string) (Process, *strings.Builder) {
	t.Helper()
	out := &strings.Builder{}
	return Process{
		Args: append([]string{}, args...), Stdout: out, Stamped: "v1.2.3", Commit: "abc1234",
		Config: config.Config{StateDir: t.TempDir()},
		Ports:  f.Ports(),
	}, out
}

func TestTheRunFunctionRunsInProcess(t *testing.T) {
	t.Parallel()
	p, _ := process(t, world(t), "check")
	code, sc := execute(t.Context(), t, p)
	said := sc.output()
	if code != exit.Done || !strings.Contains(said, "6 entries in 2 files, and no problem") {
		t.Fatalf("exit %d, said:\n%s", code, said)
	}
}

func TestACancelledRunSaysItWasStoppedAndWritesNothing(t *testing.T) {
	t.Parallel()
	f := world(t)
	p, _ := process(t, f, "gen", "--apply")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	code, sc := execute(ctx, t, p)
	said := sc.output()
	if code != exit.Failed || !strings.Contains(said, "stopped by a signal") || !strings.Contains(said, "`gragas gen --apply`") {
		t.Fatalf("exit %d, said:\n%s", code, said)
	}
	if len(f.Wrote) != 0 {
		t.Errorf("a stopped run wrote %v", f.Wrote)
	}
}

func TestADryRunWritesNothingAndEndsOnTheApplyLine(t *testing.T) {
	t.Parallel()
	f := world(t)
	p, _ := process(t, f, "gen", "-C", "some dir")
	code, sc := execute(t.Context(), t, p)
	said := sc.output()
	if code != exit.Declined || len(f.Wrote) != 0 {
		t.Fatalf("exit %d, wrote %v", code, f.Wrote)
	}
	wantLine, err := shell.Quote(shell.POSIX, []string{"gragas", "gen", "--apply", "-C", "some dir"})
	if err != nil || !strings.HasSuffix(strings.TrimSpace(said), wantLine) {
		t.Errorf("the dry run does not end on the apply line:\n%s", said)
	}
}

func TestAFailedWriteNamesTheRun(t *testing.T) {
	t.Parallel()
	f := world(t)
	f.WriteErr = errors.New("disk full")
	p, _ := process(t, f, "gen", "--apply")
	code, sc := execute(t.Context(), t, p)
	said := sc.output()
	if code != exit.Failed || !strings.Contains(said, "disk full") || !strings.Contains(said, "(run ") {
		t.Fatalf("exit %d, said:\n%s", code, said)
	}
}

func TestARunThatWritesNeedsAStateFolder(t *testing.T) {
	t.Parallel()
	p, _ := process(t, world(t), "gen", "--apply")
	p.Config.StateDir = ""
	if code, sc := execute(t.Context(), t, p); code != exit.NotReady {
		said := sc.output()
		t.Fatalf("exit %d, want not ready; said:\n%s", code, said)
	}
}

func TestJSONAnswersWithTheResult(t *testing.T) {
	t.Parallel()
	p, out := process(t, world(t), "gen", "--json")
	// What a person reads is captured here, where the binary drops it under
	// --json: the acceptance suite holds that stdout is the document alone.
	if code, sc := execute(t.Context(), t, p); code != exit.Declined {
		said := sc.output()
		t.Fatalf("exit %d, said:\n%s", code, said)
	}
	var r coreapi.Result
	if err := json.Unmarshal([]byte(out.String()), &r); err != nil {
		t.Fatalf("not one JSON document: %v\n%s", err, out)
	}
	if r.Schema != coreapi.ResultSchema || r.Command != "gen" || r.Ended != "declined" || r.Apply == nil || *r.Apply || len(r.Changes) != 4 {
		t.Errorf("answer = %+v", r)
	}
}

// TestTheWritingCommandsComeFromTheCode: every command that takes --apply
// carries the writes annotation, and only those (cli.commands.1).
func TestTheWritingCommandsComeFromTheCode(t *testing.T) {
	t.Parallel()
	s := &session{rec: &recorder{}}
	var writing []string
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		_, marked := c.Annotations[writes]
		takesApply := c.Flags().Lookup("apply") != nil
		if marked != takesApply {
			t.Errorf("%s: writes annotation %v, --apply %v", c.CommandPath(), marked, takesApply)
		}
		if marked {
			writing = append(writing, c.Name())
		}
		for _, k := range c.Commands() {
			walk(k)
		}
	}
	walk(s.newRoot(t.Context()))
	if !slices.Equal(writing, []string{"bind", "clear", "fixture", "new", "supersede", "gen", "import", "init", "recover", "migrate", "add", "approve", "record", "upgrade", "preset", "rebind", "confirm", "edit", "exclude", "include", "new", "promote", "renumber", "replace", "restore", "retire", "edit", "new", "edit", "new", "retire", "unbind", "undo"}) {
		t.Errorf("writing commands = %v, want [bind clear fixture new supersede gen import init recover migrate add approve record upgrade preset rebind confirm edit exclude include new promote renumber replace restore retire edit new edit new retire unbind undo]", writing)
	}
}

// TestEveryFlagIsSaid: help is true when every flag has words of its own
// (cli.commands.8); a flag with none would show as a bare name.
func TestEveryFlagIsSaid(t *testing.T) {
	t.Parallel()
	s := &session{rec: &recorder{}}
	n := 0
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		c.Flags().VisitAll(func(f *pflag.Flag) {
			n++
			if strings.TrimSpace(f.Usage) == "" {
				t.Errorf("%s --%s has no help text", c.CommandPath(), f.Name)
			}
		})
		for _, k := range c.Commands() {
			walk(k)
		}
	}
	walk(s.newRoot(t.Context()))
	if n == 0 {
		t.Fatal("no flag was visited")
	}
}

// TestARunThatWritesHoldsTheLockAndKeepsItsRecord: the lock is taken and
// dropped through the port, and the record holds every write, the way back
// and the ending (cli.commands.3).
func TestARunThatWritesHoldsTheLockAndKeepsItsRecord(t *testing.T) {
	t.Parallel()
	f := world(t)
	p, _ := process(t, f, "gen", "--apply")
	if code, sc := execute(t.Context(), t, p); code != exit.Done {
		said := sc.output()
		t.Fatalf("exit %d, said:\n%s", code, said)
	}
	if f.Locked != 1 || f.Released != 1 || len(f.Runs) != 1 {
		t.Fatalf("locked %d, released %d, runs %d; want one of each", f.Locked, f.Released, len(f.Runs))
	}
	r := f.Runs[0]
	if r.Code != exit.Done || r.Word != "done" || len(r.Changes) != len(f.Wrote) || r.Undo == nil {
		t.Errorf("record = %+v, wrote %v", r, f.Wrote)
	}
}

// TestAHeldLockIsBusyAndWritesNothing: another run holds the lock, so this
// one ends busy, says to run again, and writes nothing.
func TestAHeldLockIsBusyAndWritesNothing(t *testing.T) {
	t.Parallel()
	f := world(t)
	f.Held = true
	p, _ := process(t, f, "gen", "--apply")
	code, sc := execute(t.Context(), t, p)
	said := sc.output()
	if code != exit.Busy || !strings.Contains(said, "another gragas run is writing") || len(f.Wrote) != 0 || len(f.Runs) != 0 {
		t.Fatalf("exit %d, wrote %v, runs %d, said:\n%s", code, f.Wrote, len(f.Runs), said)
	}
}

// TestNothingOfTheKitIsImported: this package takes nothing (its tests apart: the plan frames and the exit codes are held to the kit) from the kit's ui
// (A70). The tree is drawn by the Draw function main fills from host/term, and
// the answers by the Terminal port.
func TestNothingOfTheKitIsImported(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	used := map[string]bool{}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := parser.ParseFile(token.NewFileSet(), f, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(src, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok {
				if id, ok := sel.X.(*ast.Ident); ok && id.Name == "ui" {
					used[sel.Sel.Name] = true
				}
			}
			return true
		})
	}
	var got []string
	for name := range used {
		got = append(got, name)
	}
	slices.Sort(got)
	if len(got) != 0 {
		t.Errorf("the package uses ui.%v, want none of the kit", got)
	}
}

// TestADryRunShowsThePlanAndEndsDeclinedOnTheApplyLine: the plan is a list of the
// files it would write, then the dry-run footer, then declined with the line that
// applies it, as one screen on standard output (cli.output.7).
func TestADryRunShowsThePlanAndEndsDeclinedOnTheApplyLine(t *testing.T) {
	t.Parallel()
	p, _ := process(t, world(t), "gen")
	code, sc := execute(t.Context(), t, p)
	if code != exit.Declined || len(sc.out.Shown) != 1 {
		t.Fatalf("exit %d, %d screens", code, len(sc.out.Shown))
	}
	bl := sc.out.Shown[0].Blocks()
	if _, ok := bl[len(bl)-2].(model.DryRunFooter); !ok {
		t.Errorf("the block before the closing word is %T, want the dry-run footer", bl[len(bl)-2])
	}
	c, ok := bl[len(bl)-1].(model.Closing)
	if !ok || c.Word() != model.Declined || c.Next() != "gragas gen --apply" {
		t.Errorf("closing = %+v, want declined with the apply line", bl[len(bl)-1])
	}
	if len(sc.diag.Said) != 1 || !strings.HasPrefix(sc.diag.Said[0], "warning: the names check is skipped") {
		t.Errorf("standard error said %q, want the names warning alone", sc.diag.Said)
	}
}

// TestUnderJSONAPersonSeesNothingOnStdoutAndTheWarningStaysOnStderr: the
// screens are not drawn, the document is the answer, and a warning still goes
// to the diagnostics.
func TestUnderJSONAPersonSeesNothingOnStdoutAndTheWarningStaysOnStderr(t *testing.T) {
	t.Parallel()
	p, _ := process(t, world(t), "check", "--json")
	code, sc := execute(t.Context(), t, p)
	if code != exit.Done || len(sc.out.Shown) != 0 || len(sc.out.Said) != 0 || len(sc.diag.Said) != 1 {
		t.Errorf("exit %d, shown %d, said %q, diagnostics %q", code, len(sc.out.Shown), sc.out.Said, sc.diag.Said)
	}
}

// TestAMistypedCommandIsAnErrorOnStderrWithTheLineToType: the usage code, the
// error and the nearest command on the diagnostics, nothing on stdout.
func TestAMistypedCommandIsAnErrorOnStderrWithTheLineToType(t *testing.T) {
	t.Parallel()
	p, _ := process(t, world(t), "chek")
	code, sc := execute(t.Context(), t, p)
	want := []string{`error: unknown command "chek" for "gragas"`, "       did you mean `gragas check`?"}
	if code != exit.Usage || !slices.Equal(sc.diag.Said, want) || len(sc.out.Shown) != 0 {
		t.Errorf("exit %d, diagnostics %q, shown %d; want the usage code and %q", code, sc.diag.Said, len(sc.out.Shown), want)
	}
}

// TestEveryEndingIsOneOfTheEightWords: the closing words the old commands end
// on, each as the screen and the answer name it.
func TestEveryEndingIsOneOfTheEightWords(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		args []string
		code int
		word model.Word
		mod  func(*porttest.Fake, *Process)
	}{
		{"check", []string{"check"}, exit.Done, model.Done, nil},
		{"not a rulebook", []string{"check"}, exit.NotReady, model.NotReady, func(f *porttest.Fake, _ *Process) { f.Repo = fstest.MapFS{} }},
		{"apply and check", []string{"gen", "--apply", "--check"}, exit.Usage, model.Usage, nil},
		{"stale check", []string{"gen", "--check"}, exit.Findings, model.Findings, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := world(t)
			p, _ := process(t, f, tc.args...)
			if tc.mod != nil {
				tc.mod(f, &p)
				p.Ports = f.Ports()
			}
			code, sc := execute(t.Context(), t, p)
			if code != tc.code || sc.closing() != tc.word {
				t.Errorf("exit %d and word %q, want %d and %q", code, sc.closing(), tc.code, tc.word)
			}
		})
	}
}

// counted is a source that counts its closes.
type counted struct {
	legacy.Source
	closed *int
}

func (c counted) Close() error {
	*c.closed++
	return nil
}

// closing is a Files whose every repository is a counted source.
type closing struct {
	legacy.Files
	closed *int
}

func (c closing) Repo(dir string) (legacy.Source, error) {
	src, err := c.Files.Repo(dir)
	if err != nil {
		return nil, err
	}
	return counted{src, c.closed}, nil
}

// TestARunLetsGoOfTheRepositoryItRead: check and gen let go of the
// repository they read once, however they end, so no folder is held open
// after a run, which Windows would refuse to remove. Planted failure: check or gen ending without release.
func TestARunLetsGoOfTheRepositoryItRead(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		args []string
		code int
		mod  func(*porttest.Fake, *Process)
	}{
		{"check", []string{"check"}, exit.Done, nil},
		{"check with findings", []string{"check"}, exit.Findings, func(f *porttest.Fake, p *Process) {
			f.NamesErr, f.Names, p.Config.NamesFile = nil, "errors\n", "names"
		}},
		{"not a rulebook", []string{"check"}, exit.NotReady, func(f *porttest.Fake, _ *Process) { f.Repo = fstest.MapFS{} }},
		{"dry run", []string{"gen"}, exit.Declined, nil},
		{"stale check", []string{"gen", "--check"}, exit.Findings, nil},
		{"apply", []string{"gen", "--apply"}, exit.Done, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := world(t)
			p, _ := process(t, f, tc.args...)
			if tc.mod != nil {
				tc.mod(f, &p)
			}
			var closed int
			p.Ports = f.Ports()
			p.Ports.Files = closing{p.Ports.Files, &closed}
			code, sc := execute(t.Context(), t, p)
			if code != tc.code || closed != 1 {
				t.Errorf("exit %d, the repository closed %d times; want exit %d, closed once. Said:\n%s", code, closed, tc.code, sc.output())
			}
		})
	}
}
