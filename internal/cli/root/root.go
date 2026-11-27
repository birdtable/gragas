// Package root is gragas's command tree and the surface around it: the
// closing words, the exit codes, --json, and the lock and run record around a
// run that writes. It renders what the application returns, through the
// Terminal port and model.Screen, and decides nothing about the rulebook.
//
// Nothing of the shared kit's ui package is imported here (A70): the tree is
// drawn by the Draw function main fills from host/term. The menu is drawn by
// the Terminal port.
package root

import (
	"context"
	"errors"
	"fmt"
	"io"
	"runtime/debug"
	"slices"
	"strings"

	"github.com/birdtable/go-console/shell"
	"github.com/birdtable/go-run/version"
	"github.com/spf13/cobra"

	"github.com/birdtable/gragas/internal/cli/kit"
	"github.com/birdtable/gragas/internal/config"
	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/exit"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
	"github.com/birdtable/gragas/internal/port/legacy"
)

// Process is what main hands a run: its arguments, after the program's
// name, and where its JSON answer goes.
type Process struct {
	Args   []string
	Stdout io.Writer
	// Stamped and Commit are what the Taskfile stamped, "" in a plain build.
	Stamped, Commit string
	Config          config.Config
	// Problem is what config.Load found wrong with the settings, nil when
	// nothing: every command ends on it before it runs, and help still
	// prints.
	Problem error
	// Ports are the adapters main filled, the only way a run reaches the
	// world.
	Ports legacy.Ports
	// World is the ports of the new program, as far as their
	// adapters are built: Tree, Clock and Terminal now. The Terminal is
	// standard output, where a person's answer goes. The old commands do not
	// read the others; they arrive with their adapters, Schema with the assets.
	World port.Ports
	// Contract is the Schema port over the contract's schemas, which a
	// document from outside is read by: a parent's bundle (A50). World's
	// Schema holds the format's. nil in a build that reads no such document.
	Contract port.Schema
	// Readers are the tool configs check reads, loaded once by
	// main through the Assets port. The zero value holds none.
	Readers coreapi.Readers
	// Tiers is the tier table the tier contract is read against,
	// loaded once by main through the Assets port. The zero value holds none.
	Tiers coreapi.TierTable
	// Diagnostics is the terminal of standard error: warnings, and the error
	// of a command line that was not understood (cli.output.7).
	Diagnostics port.Terminal
	// History makes the History port for a run, named by its id, from the git
	// the settings name. It is made per run because the adapter names the run in
	// a failure, and a run has its id only once the root claims it. It answers
	// ErrNoGit when the settings name none; nil in a build with no git.
	History func(run string) (port.History, error)
	// Draw draws the command tree; nil runs it on cobra alone, as the subset
	// build and the tests do.
	Draw Draw
	// Stopped is whether a signal the Draw listens for ended ctx, the
	// context of a command it runs, as the kit tells it by its own name
	//; main fills it from host/term with the Draw (A70).
	// Nil is no signal ever, as with a nil Draw nothing listens for one.
	Stopped func(ctx context.Context) bool
	// Dialect is the shell every line to paste is written for: the plan,
	// apply, next and way-back lines. main hands
	// the one GRAGAS_SHELL or the shell key names, or for auto the one
	// host/term's HereDialect finds; the zero value is POSIX, as the tests
	// build it. A plan's record is written for POSIX whatever it is.
	Dialect shell.Dialect
	// NoQuoting is why auto found no dialect for the shell gragas was
	// started from: host/term's HereDialect error, which wraps go-console's
	// ErrNoDialect for fish, csh and the like and holds the kit's text, which
	// names the shell; nil otherwise. Then no line to paste is printed: a
	// gragas command is said in words, and the run's word is not changed
	// Its text is shown only as %q, and not at all
	// when the screen would not show it as written (shownAs).
	NoQuoting error
}

// Draw runs the command tree once and draws it, help and the line it did not
// understand included; onError is told of an error the tree returned. main
// fills it from host/term (A70), the one package that draws with the kit; a
// build with none runs the tree on cobra alone.
type Draw func(ctx context.Context, tree *cobra.Command, onError func(io.Writer, error)) error

// ErrNoGit is the settings naming no git: programs.git is the only place the
// path to git comes from, and a program found on PATH skips the trust walk
// (A48, cli.config.12).
var ErrNoGit = errors.New("the settings name no git")

// session is one run: what it was handed and what it has done so far.
type session struct {
	p Process
	// stop ends the context the commands hold; nil where no run is made of
	// this session (the listings of the tree).
	stop context.CancelFunc
	// heard is closed once the run has said a signal stopped it; unhook
	// takes back that saying, and is false where it has begun. Both are nil
	// until the tree's first hook sets them.
	heard  chan struct{}
	unhook func() bool
	build  version.Build
	rec    *recorder
	// json is whether the run answers in JSON: stdout then holds the document
	// alone, and what a person reads is not drawn.
	json bool
	// sarif is whether the run answers in SARIF: stdout then holds the SARIF
	// document alone, and what a person reads goes to standard error.
	sarif bool
	// applied is whether the run was given --apply to write.
	applied bool
	// printed is whether a command wrote a document of its own.
	printed bool
	// data is the answer's data under --json, set by the command.
	data any
	// ended is the closing word the run drew, nil before one is; changes are
	// the lines of what was, or would be, written; fault is the last failure
	// said, and died the last error the tree said.
	ended   *ending
	changes []string
	// saidWhy is whether the run has said why a line to paste is given in
	// words (session.line), which it says once.
	saidWhy bool
	fault   string
	died    string
	// faultNext is the line to type that fault says, "" when it says none.
	faultNext string
	// finished is whether the run record is closed and the lock released.
	finished bool
	// capture is set on a replay of guard staged: the plan step hands it the
	// change and ends, planning nothing.
	capture *captured
	// crashJournal journals the refusal of a command that fails closed, should
	// it crash; a command sets it once it has a root to journal in, and it is
	// nil before.
	crashJournal func() error
}

// exitCode carries a command's exit status out through cobra, which deals in
// errors. The error handler leaves it unprinted: the command printed its own
// ending.
type exitCode int

func (e exitCode) Error() string { return fmt.Sprintf("exit status %d", int(e)) }

// exitErr is the error a command returns to end with code, one of go-run/exit's.
func exitErr(code int) error {
	if code == exit.Done {
		return nil
	}
	return exitCode(code)
}

// exitFor is the exit code of what the tree returned: the code a command
// chose, or the usage code, because the only other errors are cobra's own, a
// command line it did not understand.
func exitFor(err error) int {
	if err == nil {
		return exit.Done
	}
	if ec, ok := errors.AsType[exitCode](err); ok {
		return int(ec)
	}
	return exit.Usage
}

// Execute runs the tree once and returns the process exit code. main is the
// only caller that exits.
//
// Execute holds the program's one recover: a panic anywhere in
// the run ends it on the closing word "failed", with the trace on standard
// error alone, and a run that held the lock or a record closes it on the same
// path as on an error. A command never recovers on its own.
func Execute(ctx context.Context, p Process) (code int) {
	args := p.Args
	// The run's arguments are p.Args and nothing else. cobra reads os.Args
	// for a nil list, which in a test is the test binary's own flags,
	// so a nil list is the empty one: the bare command.
	if args == nil {
		args = []string{}
	}
	// --version alone is the version command, as other tools' is: one answer for
	// a script checking an install.
	if len(args) == 1 && args[0] == "--version" {
		args = []string{"version"}
	}
	s := &session{p: p, build: version.Read().With(version.Stamp{Version: p.Stamped, Commit: p.Commit}), json: wantsJSON(args), sarif: wantsSARIF(args)}
	s.rec = &recorder{cfg: p.Config, locks: p.Ports.Locks, runs: p.Ports.Runs, argv: append([]string{"gragas"}, args...), build: s.build.String()}
	// The commands hold ctx, and the kit's Execute cancels the context it
	// hands the tree on Ctrl+C and SIGTERM: the tree's first
	// hook ties the two (newRoot), so a run reads the stop through the ctx
	// it holds.
	ctx, s.stop = context.WithCancel(ctx)
	defer s.stop()
	var tree *cobra.Command
	defer func() {
		if v := recover(); v != nil {
			code = s.crashed(v, debug.Stack(), tree, args)
		}
	}()
	tree = s.newRoot(ctx)
	tree.SetArgs(args)
	tree.SetOut(p.Stdout)
	if s.json && s.sarif {
		code = s.twoAnswers(tree, args)
		s.finish(code)
		return code
	}
	if s.json || s.sarif {
		tree.SetOut(tree.ErrOrStderr()) // help is for a person, and stdout is the answer
	}
	s.arrive(tree, args)
	if s.json && wantsHelp(tree, args) {
		code = s.helpAsJSON(tree, args)
		s.finish(code)
		return code
	}
	err := draw(p.Draw)(ctx, tree, s.errorHandler(tree, args))
	if s.unhook != nil && !s.unhook() {
		<-s.heard // the stop line is said before the run closes, never after
	}
	code = exitFor(err)
	if s.json && !s.printed {
		s.answer(result(s, tree, args, err, code))
	}
	s.finish(code)
	return code
}

// finish closes the run record and releases the lock, once: the recorder's
// own release through Lock.Take is the only one, on the normal path and on
// the panic path alike.
func (s *session) finish(code int) {
	if s.finished {
		return
	}
	s.finished = true
	s.rec.finish(code, s.ended, s.died, s.warn)
}

// crashed ends a run a panic cut short, on the closing word "failed": it
// names the command and the run, says the trace on standard error alone, and
// gives the one JSON document under --json. It is called from the root's one
// recover, and it returns a code that is never 0: that of "failed", or of
// "refused" for a command that fails closed (kit.FailsClosed).
func (s *session) crashed(v any, trace []byte, tree *cobra.Command, args []string) int {
	name, end := crashedIn(tree, args)
	// The trace names no value; the panic's own line comes first, as Go's does.
	s.sayAside(append([]string{fmt.Sprintf("panic: %v", v)}, strings.Split(strings.TrimRight(string(trace), "\n"), "\n")...))
	what := fmt.Sprintf("`%s` hit a bug and stopped: %s", name, strings.TrimSpace(strings.SplitN(fmt.Sprint(v), "\n", 2)[0]))
	if id := s.rec.id(); id != "" {
		what += ", run " + id
	}
	if end.word == model.Refused {
		what += ", so what it was asked is refused" + s.journalCrash()
		// The two lines the hook hands the agent, as the guard's own refusal
		// says them on standard error.
		s.sayAside([]string{"refused: " + what, "instead: " + end.next})
	}
	// The sentence names the command in backticks: that is the answer's next
	// when the ending gives none.
	s.fault, s.died, end.what, end.said = what, what, what, name
	code := s.end(end)
	if s.json && !s.printed && tree != nil {
		s.answer(result(s, tree, args, nil, code))
	}
	s.finish(code)
	return code
}

// journalCrash journals the refusal of a crashed command that fails closed,
// through the journal its command handed the session once it had a root,
// and is what the ending adds: nothing when the line is written, and
// otherwise why no line is kept, a journal that could not be written or a
// crash that came before the root was open.
func (s *session) journalCrash() string {
	if s.crashJournal == nil {
		return ". No refusal is journalled: the crash came before the rulebook's root was opened"
	}
	if err := s.crashJournal(); err != nil {
		return ". The refusal could not be journalled: " + err.Error()
	}
	return ""
}

// crashedIn is the command a crashed run was in, "gragas" when the tree has
// none to name, and how it ends: failed, or refused with its line to type for
// a command whose crash is a no, so the guard's hook reads it as a no.
func crashedIn(tree *cobra.Command, args []string) (string, ending) {
	if tree == nil {
		return "gragas", ending{word: model.Failed}
	}
	c, _, err := tree.Find(args)
	if err != nil || c == nil {
		return "gragas", ending{word: model.Failed}
	}
	if next, ok := c.Annotations[kit.FailsClosed]; ok {
		return c.CommandPath(), ending{word: model.Refused, next: next}
	}
	return c.CommandPath(), ending{word: model.Failed}
}

// sayAside says each line on standard error, and stops at the first it cannot
// say: a crashed run has nowhere else to tell it.
func (s *session) sayAside(lines []string) {
	if s.p.Diagnostics == nil {
		return
	}
	for _, line := range lines {
		if err := s.p.Diagnostics.Say(line); err != nil {
			return
		}
	}
}

// draw is d, or the plain run of the tree when there is none: cobra says
// nothing of its own, and onError is how the error is told.
func draw(d Draw) Draw {
	if d != nil {
		return d
	}
	return func(ctx context.Context, tree *cobra.Command, onError func(io.Writer, error)) error {
		tree.SilenceErrors, tree.SilenceUsage = true, true
		err := tree.ExecuteContext(ctx)
		if err != nil {
			onError(tree.ErrOrStderr(), err)
		}
		return err
	}
}

// arrive draws the banner where there is one: the full banner only at help or
// with no command, and only where it paints; with any other command, one
// plain line where nothing paints and nothing where it does.
func (s *session) arrive(tree *cobra.Command, args []string) {
	if !wantsBanner(tree, args) {
		return
	}
	rich := s.p.World.Terminal.Colors() > 0
	atHelp := len(args) == 0 || wantsHelp(tree, args)
	if rich != atHelp {
		return
	}
	if err := s.p.World.Terminal.Banner(s.build.String()); err != nil {
		s.warn("the banner could not be written: %v", err)
	}
}

// Flags are the global flags that change how the run is drawn, read from the
// arguments before the terminals are built, for main.
func Flags(args []string) config.Flags {
	var f config.Flags
	for _, a := range beforeDashes(args) {
		switch a {
		case "--no-color":
			f.NoColor = true
		case "--accessible":
			f.Accessible = true
		case "--plain":
			f.Plain = true
		}
	}
	return f
}

// beforeDashes is args up to a "--".
func beforeDashes(args []string) []string {
	if i := slices.Index(args, "--"); i >= 0 {
		return args[:i]
	}
	return args
}

// wantsJSON is whether the arguments ask for --json, before any --.
func wantsJSON(args []string) bool {
	return slices.ContainsFunc(beforeDashes(args), func(a string) bool { return a == "--json" || a == "--json=true" })
}

// wantsSARIF is whether the arguments ask for --sarif, before any --. Only a
// command that lists findings takes the flag; on any other, cobra refuses it.
func wantsSARIF(args []string) bool {
	return slices.ContainsFunc(beforeDashes(args), func(a string) bool { return a == "--sarif" || a == "--sarif=true" })
}

// twoAnswers ends a run given both --json and --sarif: each asks for the
// run's one document on stdout, so neither is written, and stdout stays empty.
func (s *session) twoAnswers(tree *cobra.Command, args []string) int {
	s.json = false
	see := "gragas --help"
	if c, _, err := tree.Find(args); err == nil && c != tree {
		see = c.CommandPath() + " --help"
	}
	s.die("--json and --sarif cannot be given together: each is the run's one document on stdout\nsee `%s`", see)
	return exit.Usage
}

// wantsHelp is a run that prints help: --help or -h before any --, or the
// help command.
func wantsHelp(tree *cobra.Command, args []string) bool {
	if len(args) > 0 && args[0] == "help" {
		return true
	}
	if _, _, err := tree.Find(args); err != nil {
		return false
	}
	return slices.ContainsFunc(beforeDashes(args), func(a string) bool { return a == "--help" || a == "-h" })
}

// wantsBanner is false where the output is for a program (--json, shell
// completion), for version, which says the build itself, and for a command
// line that names no command, whose stdout stays empty.
func wantsBanner(tree *cobra.Command, args []string) bool {
	if len(args) > 0 && slices.Contains([]string{"completion", cobra.ShellCompRequestCmd, cobra.ShellCompNoDescRequestCmd}, args[0]) {
		return false
	}
	if wantsJSON(args) || wantsSARIF(args) {
		return false
	}
	if wantsHelp(tree, args) {
		return true
	}
	c, rest, err := tree.Find(args)
	if err != nil || (c != tree && c.Name() == "version") {
		return false
	}
	if slices.ContainsFunc(beforeDashes(args), func(a string) bool { return strings.HasPrefix(a, "-") && !knownFlag(c, a) }) {
		return false
	}
	return c.Args == nil || c.Args(c, positional(c, rest)) == nil
}

// positional is rest without its flags and their values, as c would parse it.
func positional(c *cobra.Command, rest []string) []string {
	var out []string
	for i := 0; i < len(rest); i++ {
		a := rest[i]
		if a == "--" {
			return append(out, rest[i+1:]...)
		}
		if !strings.HasPrefix(a, "-") || a == "-" {
			out = append(out, a)
			continue
		}
		name, _, inline := strings.Cut(strings.TrimLeft(a, "-"), "=")
		f := c.Flags().Lookup(name)
		if f == nil && len(name) == 1 {
			f = c.Flags().ShorthandLookup(name)
		}
		if f == nil {
			f = c.InheritedFlags().Lookup(name)
		}
		if f != nil && !inline && f.NoOptDefVal == "" {
			i++ // the flag's value is the next word
		}
	}
	return out
}

// knownFlag is whether a, a --name or -x, is a flag c takes.
func knownFlag(c *cobra.Command, a string) bool {
	name, _, _ := strings.Cut(strings.TrimLeft(a, "-"), "=")
	if strings.HasPrefix(a, "--") {
		return c.Flags().Lookup(name) != nil || c.InheritedFlags().Lookup(name) != nil
	}
	return len(name) == 1 && (c.Flags().ShorthandLookup(name) != nil || c.InheritedFlags().ShorthandLookup(name) != nil)
}
