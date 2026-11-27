package git

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/birdtable/go-proc/proc"

	"github.com/birdtable/go-files/filestest"
)

// A stand-in for git is this test binary, copied into a test's folder with
// the rules it answers by in rulesFile beside it:
// TestMain finds the file and answers by it in place of running the tests.
// A shell script in its place runs on a Unix alone.
const rulesFile = "standin.json"

// Answer is what the stand-in does, in this order: it writes Mark, hangs,
// runs Run, prints Out and Zeros NUL bytes on stdout, then Err and ErrZeros
// NUL bytes on stderr, writes its environment to EnvTo, and exits with Code.
type Answer struct {
	// Mark is a file the stand-in writes first, so a test knows the program
	// started before its deadline came.
	Mark string `json:"mark,omitempty"`
	// Hang sleeps a minute, as a git that never ends, before anything else.
	Hang bool `json:"hang,omitempty"`
	// Run is a program run first with the same arguments, input and
	// environment, whose output is the stand-in's; when it fails, the
	// stand-in exits with its code.
	Run      string `json:"run,omitempty"`
	Out      string `json:"-"`
	Zeros    int    `json:"zeros,omitempty"`
	Err      string `json:"-"`
	ErrZeros int    `json:"errZeros,omitempty"`
	// EnvTo is a file the stand-in writes the environment it was handed
	// to, sorted, each entry ended by \001, so no output of a run holds
	// it: a test reads the file and says what differs by name alone.
	EnvTo string `json:"envTo,omitempty"`
	Code  int    `json:"code,omitempty"`
}

// answerBytes is an Answer in the rules file: Out and Err as bytes, so
// bytes that are not UTF-8 reach the stand-in unchanged.
type answerBytes struct {
	plainAnswer
	Out []byte `json:"out,omitempty"`
	Err []byte `json:"err,omitempty"`
}

type plainAnswer Answer

// MarshalJSON writes a as the rules file holds it.
func (a Answer) MarshalJSON() ([]byte, error) {
	return json.Marshal(answerBytes{plainAnswer(a), []byte(a.Out), []byte(a.Err)})
}

// UnmarshalJSON reads a as the rules file holds it.
func (a *Answer) UnmarshalJSON(b []byte) error {
	var w answerBytes
	if err := json.Unmarshal(b, &w); err != nil {
		return err
	}
	*a = Answer(w.plainAnswer)
	a.Out, a.Err = string(w.Out), string(w.Err)
	return nil
}

// Rule matches when the arguments, joined by spaces with a space before and
// after, hold each of Has; joined by spaces alone, end with End; and the
// working folder holds the file If.
type Rule struct {
	Has []string `json:"has,omitempty"`
	End string   `json:"end,omitempty"`
	If  string   `json:"if,omitempty"`
	// Stdin, when set, is a text the run's standard input must hold.
	Stdin  string `json:"stdin,omitempty"`
	Answer Answer `json:"answer"`
}

// Always is the rule that answers every run a.
func Always(a Answer) Rule { return Rule{Answer: a} }

// StandIn writes a stand-in for git into dir, answering by rules, and
// answers its path. go test names this binary git.test and the suffix the
// system runs a program by; the copy is git with that suffix, so the system
// runs it, a lookup by name finds it, and no test names the suffix itself.
func StandIn(t testing.TB, dir string, rules ...Rule) string {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	suffix, ok := strings.CutPrefix(filepath.Base(self), "git.test")
	if !ok {
		t.Fatalf("this test binary is %s, not git.test as go test names it, and the stand-in takes its name from that", filepath.Base(self))
	}
	b, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := json.Marshal(rules)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, rulesFile), doc, 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "git"+suffix)
	filestest.WriteProgram(t, path, b)
	return path
}

// StandInMain is what TestMain runs first, a child's entry that answers at
// once from os.Args: a run of the tests is git.test by name, and it is
// false there, reading nothing, and the tests run. A stand-in, a copy named
// git and the suffix, answers by the first rule of the rules file beside it
// that matches the arguments, or exits 9 when none does, and is the exit
// code, with true.
func StandInMain() (int, bool) {
	if len(os.Args) == 0 || strings.HasPrefix(filepath.Base(os.Args[0]), "git.test") {
		return 0, false
	}
	self, err := os.Executable()
	if err != nil {
		os.Stderr.WriteString("stand-in: " + err.Error() + "\n")
		return 125, true
	}
	doc, err := os.ReadFile(filepath.Join(filepath.Dir(self), rulesFile))
	if err != nil {
		os.Stderr.WriteString("stand-in: " + err.Error() + "\n")
		return 125, true
	}
	var rules []Rule
	if err := json.Unmarshal(doc, &rules); err != nil {
		os.Stderr.WriteString("stand-in: " + err.Error() + "\n")
		return 125, true
	}
	args := os.Args[1:]
	line := strings.Join(args, " ")
	var in *string
	for _, r := range rules {
		if r.Stdin != "" {
			if in == nil {
				b, _ := io.ReadAll(os.Stdin)
				s := string(b)
				in = &s
			}
			if !strings.Contains(*in, r.Stdin) {
				continue
			}
		}
		if r.matches(line) {
			return r.Answer.do(args), true
		}
	}
	return 9, true
}

func (r Rule) matches(line string) bool {
	for _, h := range r.Has {
		if !strings.Contains(" "+line+" ", h) {
			return false
		}
	}
	if !strings.HasSuffix(line, r.End) {
		return false
	}
	if r.If != "" {
		if _, err := os.Stat(r.If); err != nil {
			return false
		}
	}
	return true
}

// do is the stand-in's run of a, with the arguments it was given.
func (a Answer) do(args []string) int {
	if a.Mark != "" {
		if err := os.WriteFile(a.Mark, []byte("started\n"), 0o600); err != nil {
			os.Stderr.WriteString("stand-in: " + err.Error() + "\n")
			return 125
		}
	}
	if a.Hang {
		time.Sleep(time.Minute)
	}
	if a.Run != "" {
		res, err := proc.Run(context.Background(), proc.Cmd{
			Path: a.Run, Args: args, Env: os.Environ(), Stdin: os.Stdin,
			Timeout: time.Minute, MaxOutput: 64 << 20, Drain: 10 * time.Second,
		})
		os.Stdout.Write(res.Stdout)
		os.Stderr.Write(res.Stderr)
		if err != nil {
			os.Stderr.WriteString("stand-in: " + err.Error() + "\n")
			return 125
		}
		if res.Code != 0 {
			return res.Code
		}
	}
	os.Stdout.WriteString(a.Out)
	os.Stdout.Write(make([]byte, a.Zeros))
	os.Stderr.WriteString(a.Err)
	os.Stderr.Write(make([]byte, a.ErrZeros))
	if a.EnvTo != "" {
		env := os.Environ()
		slices.Sort(env)
		var b strings.Builder
		for _, kv := range env {
			b.WriteString(kv + "\x01")
		}
		if err := os.WriteFile(a.EnvTo, []byte(b.String()), 0o600); err != nil {
			os.Stderr.WriteString("stand-in: " + err.Error() + "\n")
			return 125
		}
	}
	return a.Code
}
