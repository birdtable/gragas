package tool_test

import (
	"testing"
	"time"

	"github.com/birdtable/go-testkit/fake"

	configtest "github.com/birdtable/gragas/internal/config/testing"
)

// libraryStep is the step of go-testkit's fake that does what the configtest step s does, and
// whether there is one. A step that holds one field, or the pair or the branches a field names
// with it, has one when the library has the step; a step that sets two fields at once, or one
// only gragas's stand-in does, has none, and its program stays on configtest.Fake.
func libraryStep(s configtest.Step) (fake.Step, bool) {
	switch {
	case s.Match != "":
		then, ok := libraryList(s.Then)
		return fake.When(s.Match, then...), ok && s.Out == nil && s.IfExists == ""
	case s.IfExists != "":
		then, okThen := libraryList(s.Then)
		els, okElse := libraryList(s.Else)
		return fake.IfExists(s.IfExists, then, els), okThen && okElse
	case s.Version != "":
		// A --version first argument says the line and ends 0, after the mark is written.
		var then []fake.Step
		if s.Mark != "" {
			then = append(then, fake.WriteFile(s.Mark, "x"))
		}
		return fake.When("--version*", append(then, fake.Out(s.Version+"\n"))...), s.Out == nil
	case s.Out != nil:
		return fake.Out(*s.Out), onlyOne(s)
	case s.Err != nil:
		return fake.Err(*s.Err), onlyOne(s)
	case s.Exit != nil:
		return fake.Exit(*s.Exit), onlyOne(s)
	case s.Write != "":
		return fake.WriteFile(s.Write, s.Text), s.Under == "" && !s.Append && !s.Args && !s.First && !s.Env && onlyOne(s)
	case s.PrintArgs:
		return fake.PrintArgs(), onlyOne(s)
	case len(s.Vars) > 0:
		return fake.PrintEnv(s.Vars...), onlyOne(s)
	case s.EnvOut:
		return fake.PrintEnv(), onlyOne(s)
	case s.LogArgs != "":
		return fake.AppendArgs(s.LogArgs), onlyOne(s)
	case s.Spin:
		return fake.Spin(), onlyOne(s)
	case s.Flood != "":
		return fake.Flood(s.Flood), onlyOne(s)
	case s.HoldMiB > 0:
		return fake.Hold(s.HoldMiB), onlyOne(s)
	case s.SleepMS > 0:
		return fake.Sleep(time.Duration(s.SleepMS) * time.Millisecond), onlyOne(s)
	}
	return fake.Step{}, false
}

// onlyOne is whether s holds nothing but the one field libraryStep read: the library's steps each
// do one thing, and a configtest step that does two would do them in an order of its own.
func onlyOne(s configtest.Step) bool {
	n := 0
	for _, set := range []bool{
		s.Out != nil, s.Err != nil, s.Write != "", s.Append, s.Args, s.First, s.Env, s.PID != "",
		len(s.Run) > 0, s.Exec != "", s.SleepMS > 0, s.Exit != nil, s.OnlyFirst != "", s.Version != "",
		s.Mark != "", s.PrintArgs, s.LogArgs != "", s.CatLast != "", len(s.Vars) > 0, s.Spin,
		s.Flood != "", s.HoldMiB > 0, s.EnvOut, s.Script, s.EmptyCwd, s.Stdin, s.Under != "",
		s.Lock != "", s.Stuck, len(s.Raw) > 0, s.Zero > 0, s.ZeroErr > 0, s.EnvErr, s.Text != "",
	} {
		if set {
			n++
		}
	}
	// Text goes with Write: a Write step with text is one thing.
	if s.Write != "" && s.Text != "" {
		n--
	}
	return n == 1
}

// libraryList is libraryStep of each step, and whether every one has a step.
func libraryList(steps []configtest.Step) ([]fake.Step, bool) {
	out := make([]fake.Step, 0, len(steps))
	for _, s := range steps {
		l, ok := libraryStep(s)
		if !ok {
			return nil, false
		}
		out = append(out, l)
	}
	return out, true
}

// place puts a built program at folder/name that does steps: go-testkit's fake.Program when every
// step has a library step, and configtest.Fake, which has gragas's own, when one has none.
func place(t testing.TB, folder, name string, steps ...configtest.Step) string {
	t.Helper()
	if lib, ok := libraryList(steps); ok {
		return fake.Program(t, folder, name, lib...)
	}
	return configtest.Fake(t, folder, name, steps...)
}
