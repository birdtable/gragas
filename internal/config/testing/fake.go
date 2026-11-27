package configtest

import (
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/birdtable/go-testkit/fake"
)

// A Step is one thing a Fake program does when it is run; a
// Fake does its steps in order. The fields are those of testdata/fake's steps,
// which say what each does. A nil Out or Err writes nothing, so a step that
// writes the empty string is not the same as none.
type Step struct {
	Out       *string  `json:"out,omitempty"`
	Err       *string  `json:"err,omitempty"`
	Write     string   `json:"write,omitempty"`
	Text      string   `json:"text,omitempty"`
	Append    bool     `json:"append,omitempty"`
	Args      bool     `json:"args,omitempty"`
	First     bool     `json:"first,omitempty"`
	Env       bool     `json:"env,omitempty"`
	PID       string   `json:"pid,omitempty"`
	Run       []string `json:"run,omitempty"`
	Exec      string   `json:"exec,omitempty"`
	SleepMS   int      `json:"sleep_ms,omitempty"`
	Exit      *int     `json:"exit,omitempty"`
	OnlyFirst string   `json:"only_first,omitempty"`
	Version   string   `json:"version,omitempty"`
	Mark      string   `json:"mark,omitempty"`
	PrintArgs bool     `json:"print_args,omitempty"`
	LogArgs   string   `json:"log_args,omitempty"`
	CatLast   string   `json:"cat_last,omitempty"`
	Vars      []string `json:"vars,omitempty"`
	VarsTo    string   `json:"vars_to,omitempty"`
	Spin      bool     `json:"spin,omitempty"`
	Flood     string   `json:"flood,omitempty"`
	HoldMiB   int      `json:"hold_mib,omitempty"`
	EnvOut    bool     `json:"env_out,omitempty"`
	Script    bool     `json:"script_args,omitempty"`
	EmptyCwd  bool     `json:"empty_cwd,omitempty"`
	Stdin     bool     `json:"stdin,omitempty"`
	Under     string   `json:"under,omitempty"`
	Lock      string   `json:"lock,omitempty"`
	Stuck     bool     `json:"stuck,omitempty"`
	Raw       []byte   `json:"raw,omitempty"`
	Zero      int      `json:"zero,omitempty"`
	ZeroErr   int      `json:"zero_err,omitempty"`
	EnvErr    bool     `json:"env_err,omitempty"`
	Match     string   `json:"match,omitempty"`
	IfExists  string   `json:"if_exists,omitempty"`
	Then      []Step   `json:"then,omitempty"`
	Else      []Step   `json:"else,omitempty"`
}

// Case is a step that, when the program's arguments joined by spaces fit
// pattern (where * is any run of characters), does then and ends the program
// with their exit code or 0; the first case that fits wins, as in a shell's case.
func Case(pattern string, then ...Step) Step { return Step{Match: pattern, Then: then} }

// If is a step that does then when path exists in the folder the program runs
// in, else els, and goes on after.
func If(path string, then, els []Step) Step { return Step{IfExists: path, Then: then, Else: els} }

// Version is a step that answers --version with line and ends 0, writing a
// mark file at mark first when it is not "".
func Version(line, mark string) Step { return Step{Version: line, Mark: mark} }

// PrintArgs is a step that writes each argument on a line of its own.
func PrintArgs() Step { return Step{PrintArgs: true} }

// LogArgs is a step that appends the program's arguments, joined by spaces, as a
// line to the file at path.
func LogArgs(path string) Step { return Step{LogArgs: path} }

// CatLast is a step that appends "config <last argument>" to the file at path,
// then the lines of the file the last argument names: a program that is handed
// the config to read and logs what it was given.
func CatLast(path string) Step { return Step{CatLast: path} }

// Vars is a step that writes the values of the named environment variables
// to stdout joined by "|", "unset" for one the program was not given.
func Vars(names ...string) Step { return Step{Vars: names} }

// VarsTo is Vars written to the file at path in place of stdout: what a run
// was given, read where its own output is not seen, as a version probe's.
func VarsTo(path string, names ...string) Step { return Step{Vars: names, VarsTo: path} }

// EnvOut is a step that writes the program's environment to stdout, sorted,
// a NAME=value to a line.
func EnvOut() Step { return Step{EnvOut: true} }

// ScriptArgs is a step that writes script=<first argument> and then the other
// arguments, one to a line: what a program that is handed its script first says.
func ScriptArgs() Step { return Step{Script: true} }

// EmptyCwd is a step that ends the program with code 7 when the folder it runs
// in holds anything.
func EmptyCwd() Step { return Step{EmptyCwd: true} }

// Stdin is a step that copies the program's standard input to its stdout, to
// its end: a program that shows what it was handed to read.
func Stdin() Step { return Step{Stdin: true} }

// Spin is a step that never ends: a busy loop.
func Spin() Step { return Step{Spin: true} }

// Flood is a step that writes s to stdout over and over until the reader ends it.
func Flood(s string) Step { return Step{Flood: s} }

// Memory is a step that fills and keeps mib MiB of memory.
func Memory(mib int) Step { return Step{HoldMiB: mib} }

// WriteUnder is a step that makes the folder the environment variable env
// names and writes text to the file name in it; the program exits 9 when
// the variable is empty or the write fails.
func WriteUnder(env, name, text string) Step { return Step{Under: env, Write: name, Text: text} }

// StuckUnder is a step that makes the folder name in the folder the variable env
// names, puts a file in it and makes it read-only (0555), so a removal of the
// folder above it fails with "permission denied": what a program leaves that a
// run cannot clear. The program exits 9 when it cannot.
func StuckUnder(env, name string) Step { return Step{Under: env, Write: name, Text: "x", Stuck: true} }

// LockUnder is a step that makes the folder name in the folder the variable
// env names, waits ms milliseconds and removes it again; the program exits 3
// when the folder is already there, so two runs at once are told apart.
func LockUnder(env, name string, ms int) Step { return Step{Under: env, Lock: name, SleepMS: ms} }

// Out is a step that writes s to the program's stdout.
func Out(s string) Step { return Step{Out: &s} }

// Err is a step that writes s to the program's stderr.
func Err(s string) Step { return Step{Err: &s} }

// Exit is a step that ends the program with code.
func Exit(code int) Step { return Step{Exit: &code} }

// Exec is a step that runs the program at path with the program's own arguments and standard streams,
// then goes on; a non-zero exit of it ends the program with the same code (fake.Exec).
func Exec(path string) Step { return Step{Exec: path} }

// fakeSource is the stand-in program's folder: go-testkit's fake.Main with the steps only gragas has as
// handlers; fake.From builds it once for the test binary's run, whoever asks.
const fakeSource = "internal/config/testing/testdata/fake"

// libraryOf is the steps of steps as go-testkit's fake package has them (gragas#2282).
func libraryOf(t testing.TB, steps []Step) []fake.Step {
	t.Helper()
	var l []fake.Step
	for _, s := range steps {
		l = append(l, s.library(t)...)
	}
	return l
}

// custom is the Custom step of testdata/fake named name, with arg as its JSON.
func custom(t testing.TB, name string, arg any) fake.Step {
	t.Helper()
	raw, err := json.Marshal(arg)
	if err != nil {
		t.Fatal(err)
	}
	return fake.Custom(name, string(raw))
}

// library is the steps of s as go-testkit's fake package has them, in the order the program did them
// (gragas#2282): a step of the library where it has one, a Custom step of testdata/fake where only gragas
// has it. A Step stays as it is for the callers not yet on the library's names.
func (s Step) library(t testing.TB) []fake.Step {
	t.Helper()
	var out []fake.Step
	if s.OnlyFirst != "" {
		out = append(out, fake.Custom("only-first", s.OnlyFirst))
	}
	if s.Match != "" {
		return append(out, fake.When(s.Match, libraryOf(t, s.Then)...))
	}
	if s.IfExists != "" {
		return append(out, fake.IfExists(s.IfExists, libraryOf(t, s.Then), libraryOf(t, s.Else)))
	}
	out = append(out, s.reads(t)...)
	out = append(out, s.writes(t)...)
	out = append(out, s.runs(t)...)
	return out
}

// reads is what a step says or looks at: the arguments, the environment, the folder, the standard input.
func (s Step) reads(t testing.TB) []fake.Step {
	t.Helper()
	var out []fake.Step
	if s.Version != "" {
		out = append(out, custom(t, "version", map[string]string{"Line": s.Version, "Mark": s.Mark}))
	}
	if s.LogArgs != "" {
		out = append(out, fake.AppendArgs(s.LogArgs))
	}
	if s.CatLast != "" {
		out = append(out, fake.Custom("cat-last", s.CatLast))
	}
	if s.PrintArgs {
		out = append(out, fake.PrintArgs())
	}
	if len(s.Vars) > 0 && s.VarsTo != "" {
		out = append(out, custom(t, "vars-to", map[string]any{"Path": s.VarsTo, "Names": s.Vars}))
	} else if len(s.Vars) > 0 {
		out = append(out, fake.PrintEnv(s.Vars...))
	}
	if s.EmptyCwd {
		out = append(out, fake.Custom("empty-cwd", ""))
	}
	if s.Stdin {
		out = append(out, fake.Custom("stdin", ""))
	}
	if s.Script {
		out = append(out, fake.Custom("script", ""))
	}
	if s.EnvOut {
		out = append(out, fake.PrintEnv())
	}
	return out
}

// writes is what a step puts out: under a folder, to the streams, to a file.
func (s Step) writes(t testing.TB) []fake.Step {
	t.Helper()
	var out []fake.Step
	if s.Under != "" {
		out = append(out, custom(t, "under", map[string]any{"Env": s.Under, "Write": s.Write, "Text": s.Text, "Lock": s.Lock, "MS": s.SleepMS, "Stuck": s.Stuck}))
		s.Write, s.SleepMS = "", 0
	}
	if s.HoldMiB > 0 {
		out = append(out, fake.Hold(s.HoldMiB))
	}
	if s.Out != nil {
		out = append(out, fake.Out(*s.Out))
	}
	if s.Err != nil {
		out = append(out, fake.Err(*s.Err))
	}
	out = append(out, s.bytes()...)
	if s.PID != "" {
		out = append(out, fake.WritePID(s.PID))
	}
	return append(out, s.file(t)...)
}

// bytes is the steps that write bytes, not text.
func (s Step) bytes() []fake.Step {
	var out []fake.Step
	if len(s.Raw) > 0 {
		out = append(out, fake.OutBytes(s.Raw))
	}
	if s.Zero > 0 {
		out = append(out, fake.OutBytes(make([]byte, s.Zero)))
	}
	if s.ZeroErr > 0 {
		out = append(out, fake.Custom("zero-err", strconv.Itoa(s.ZeroErr)))
	}
	if s.EnvErr {
		out = append(out, fake.Custom("env-err", ""))
	}
	return out
}

// file is the step that writes a file: text, or the arguments, the first argument or the environment.
func (s Step) file(t testing.TB) []fake.Step {
	t.Helper()
	if s.Write == "" || s.Under != "" {
		return nil
	}
	switch {
	case s.Env:
		return []fake.Step{custom(t, "write-args", map[string]any{"Path": s.Write, "What": "env", "Append": s.Append})}
	case s.Args:
		return []fake.Step{custom(t, "write-args", map[string]any{"Path": s.Write, "What": "args", "Append": s.Append})}
	case s.First:
		return []fake.Step{custom(t, "write-args", map[string]any{"Path": s.Write, "What": "first", "Append": s.Append})}
	case s.Append:
		return []fake.Step{fake.AppendFile(s.Write, s.Text)}
	}
	return []fake.Step{fake.WriteFile(s.Write, s.Text)}
}

// runs is what a step starts, waits for or ends with.
func (s Step) runs(t testing.TB) []fake.Step {
	t.Helper()
	var out []fake.Step
	if len(s.Run) > 0 {
		out = append(out, custom(t, "run-path", s.Run))
	}
	if s.Exec != "" {
		out = append(out, fake.Exec(s.Exec))
	}
	if s.Flood != "" {
		out = append(out, fake.Flood(s.Flood))
	}
	if s.Spin {
		out = append(out, fake.Spin())
	}
	if s.SleepMS > 0 && s.Under == "" {
		out = append(out, fake.Sleep(time.Duration(s.SleepMS)*time.Millisecond))
	}
	if s.Exit != nil {
		out = append(out, fake.Exit(*s.Exit))
	}
	return out
}

// Fake puts a built program in folder under name, which does steps when it is
// run, and is the path of it: the same on every system, with .exe added where
// the system asks. It is go-testkit's fake.From(fakeSource).Program (gragas#2282), the stand-in for a shell
// script a test wrote to be run: no test writes a program in a shell's
// language, which Windows has none of at /bin/sh. The folder is the test's
// own, and name is one file name in it.
func Fake(t testing.TB, folder, name string, steps ...Step) string {
	t.Helper()
	return fake.From(fakeSource).Program(t, folder, name, libraryOf(t, steps)...)
}
