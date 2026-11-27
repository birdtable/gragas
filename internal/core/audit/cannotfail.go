package audit

import (
	"regexp"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// Verdict is what reading one member's command says of whether it can fail.
type Verdict struct {
	// Why is the form that makes the command unable to fail, "" when none was
	// found. It names the form, never the whole command.
	Why string
	// Unjudged is why the command could not be judged, "" when it could: it
	// did not parse, or a statement of it is not one the check reads and nothing
	// else in the member settles that it can fail. A command that is unjudged is
	// reported as not checked, never passed.
	Unjudged string
}

// CannotFail reads the text of a tier member the way go-task 3.54.0 runs it:
// by mvdan.cc/sh, the shell go-task's own interpreter is, with errexit on and
// pipefail off unless pipefail says the Taskfile turned it on (a set: holding
// pipefail on the file, the task or the member). It reads only the forms below
// and says "not checked" of every other statement that is not a plain command,
// because a reading that guesses at shell semantics passes the next spelling it
// did not think of, and a "not checked" fails closed. The statements at the top
// level are read in order, as that shell runs them; the member cannot fail when
// no statement can end the shell and the last one cannot be non-zero, and the
// shell stops at the end, at `exit 0` or after `set -n`.
//
// What is read:
//
//   - a plain command, a simple command with a literal command word: it can
//     fail, and with errexit on a failure ends the shell;
//   - `true`, `:`, `echo` and `printf` (redirects only to a file descriptor or
//     /dev/null), and an assignment: they cannot fail;
//   - `exit 0` and `exit <n>` with a literal n written without a leading zero:
//     `exit 0` ends the shell green, any other ends it red whatever errexit is;
//   - a `set` with one option, each on its own: `-e`, `+e`, `-n`, `-o errexit`,
//     `+o errexit`, `-o noexec`, `-o pipefail`, `+o pipefail`;
//   - `X || Y` with Y `true`, `:`, `echo`, `printf` or `exit 0`, which cannot
//     fail, or `exit <n>`, which fails when X can; an exit other than `exit 0`
//     anywhere in X ends the shell before Y is reached (the empty-run guard
//     `grep ... && exit 1 || true` fails);
//   - `X && Y` with Y a plain command, which ends the shell on Y, and any list
//     holding an exit other than `exit 0`;
//   - a pipeline whose last command is `cat` or `tee`, with no pipefail, whose
//     status is the last command's; and one whose last command is a plain
//     command, which ends the shell on it.
//
// Not read, so unjudged unless a statement elsewhere ends the shell red (which
// settles that the member can fail): any other `set` (a combined flag, several
// options, `--`, `-u`), `exit 00` or an exit whose status is not a literal, an
// `&&` list ending in a command that cannot fail, a negation, a statement in the
// background, a redirect to a file, a command word that is not a literal, and a
// compound command or a function. An option set again after a statement that may
// have failed is not read either. A form that would stop the shell green or turn
// errexit off, standing in an `set` the check does not read or inside a
// compound command, makes the member unjudged at once. Text that does not parse
// is unjudged.
func CannotFail(text string, pipefail bool) Verdict {
	f, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(text), "")
	if err != nil {
		return Verdict{Unjudged: "the command does not parse as shell"}
	}
	r := reading{errexit: true, pipefail: pipefail}
	for _, s := range f.Stmts {
		if v, done := r.read(classify(s, r.pipefail)); done {
			return v
		}
	}
	switch {
	case r.pending != "":
		return Verdict{Unjudged: r.pending}
	case r.lastFails:
		return Verdict{}
	}
	return Verdict{Why: r.why}
}

// reading is the state of the shell as the statements are read in order.
type reading struct {
	errexit, pipefail bool
	lastFails         bool
	why, pending      string
}

// read takes one statement's effect and says whether the verdict is already
// settled.
func (r *reading) read(e effect) (Verdict, bool) {
	switch e.kind {
	case kindExitZero, kindNoexec:
		if r.pending != "" {
			return Verdict{Unjudged: r.pending}, true
		}
		r.name(e.form)
		return Verdict{Why: r.why}, true
	case kindErrexitOff:
		r.errexit, r.lastFails = false, false
		r.name(e.form)
	case kindErrexitOn:
		r.setAgain(!r.errexit)
		r.errexit = true
	case kindPipefailOn:
		r.setAgain(true)
		r.pipefail = true
	case kindPipefailOff:
		r.setAgain(true)
		r.pipefail = false
	default:
		return r.statement(e)
	}
	return Verdict{}, false
}

// setAgain notes an option set after a statement that may have failed, which the
// check does not read: the member's last status depends on which came first.
func (r *reading) setAgain(changes bool) {
	if changes && r.lastFails {
		r.pending = "an option is set again after a statement that may have failed"
	}
	r.lastFails = false
}

// statement reads an ordinary statement.
func (r *reading) statement(e effect) (Verdict, bool) {
	switch {
	case e.unknown != "" && e.stops:
		return Verdict{Unjudged: e.unknown}, true
	case e.unknown != "":
		r.pending, r.lastFails = e.unknown, true
		return Verdict{}, false
	case e.exitNonZero || (r.errexit && e.abort):
		return Verdict{}, true
	}
	r.lastFails = e.fails
	if e.form != "" {
		r.name(e.form)
	}
	return Verdict{}, false
}

func (r *reading) name(form string) {
	if r.why == "" && form != "" {
		r.why = form
	}
}

// kind is what one top-level statement does to the shell.
type kind int

const (
	// kindStmt is an ordinary statement, placed by abort and fails.
	kindStmt kind = iota
	// kindExitZero ends the shell with status 0.
	kindExitZero
	// kindNoexec makes the shell read the rest and run none of it.
	kindNoexec
	// kindErrexitOff and kindErrexitOn turn errexit off and on.
	kindErrexitOff
	kindErrexitOn
	// kindPipefailOn and kindPipefailOff turn pipefail on and off.
	kindPipefailOn
	kindPipefailOff
)

// effect is what a statement does. abort says it can end the shell with a
// non-zero status while errexit is on, exitNonZero that it does so whatever
// errexit is (an exit other than 0), and fails that its status may be non-zero
// when it completes. form names what keeps it from failing, "" where nothing
// does. unknown says the check does not read it, and stops that a form that
// would stop the shell green, or turn errexit off, may stand in it.
type effect struct {
	kind               kind
	abort, exitNonZero bool
	fails              bool
	form               string
	unknown            string
	stops              bool
}

// failing is a plain command: it can fail and a failure ends the shell.
func failing() effect { return effect{abort: true, fails: true} }

// green is a statement that cannot fail.
func green(form string) effect { return effect{form: form} }

// unread is a statement the check does not read.
func unread(why string) effect { return effect{unknown: why} }

// classify reads one statement.
func classify(s *syntax.Stmt, pipefail bool) effect {
	switch {
	case s.Negated:
		return unread("a negation, which the check does not read")
	case s.Background:
		return unread("a statement in the background, which the check does not read")
	}
	return classifyCmd(s, pipefail)
}

func classifyCmd(s *syntax.Stmt, pipefail bool) effect {
	switch c := s.Cmd.(type) {
	case *syntax.CallExpr:
		return callEffect(c, s.Redirs)
	case *syntax.BinaryCmd:
		switch c.Op {
		case syntax.OrStmt:
			return orEffect(c, pipefail)
		case syntax.AndStmt:
			return andEffect(c, pipefail)
		case syntax.Pipe, syntax.PipeAll:
			return pipeEffect(c, pipefail)
		}
	case *syntax.IfClause, *syntax.ForClause, *syntax.WhileClause, *syntax.Subshell, *syntax.Block, *syntax.CaseClause, *syntax.FuncDecl:
		if why := unjudgedIn(s, pipefail); why != "" {
			return effect{unknown: why, stops: true}
		}
		return unread("a compound command or a function, which the check does not read")
	}
	return unread("a statement the check does not read")
}

// unjudgedIn says why a compound command makes the member unjudged at once: a
// form that stops the shell green, or turns errexit off, stands somewhere inside
// it, where whether it runs cannot be told. "" when none does.
func unjudgedIn(s *syntax.Stmt, pipefail bool) string {
	form := ""
	syntax.Walk(s, func(n syntax.Node) bool {
		sub, ok := n.(*syntax.Stmt)
		if !ok || sub == s || form != "" {
			return form == ""
		}
		e := classifyCmd(sub, pipefail)
		switch {
		case e.kind == kindExitZero, e.kind == kindNoexec, e.kind == kindErrexitOff:
			form = e.form
		case e.kind == kindStmt && e.unknown == "" && !e.fails && e.form != "":
			form = e.form
		}
		return form == ""
	})
	if form == "" {
		return ""
	}
	return form + " stands inside a compound command or a function, where whether it runs cannot be told"
}

// cannotFailWord says a command word is one whose status is 0: true, :, echo
// and printf.
func cannotFailWord(w string) bool {
	switch w {
	case "true", ":", "echo", "printf":
		return true
	}
	return false
}

// quietRedirs says every redirect goes to a file descriptor or /dev/null, so it
// cannot make a command that cannot fail do so.
func quietRedirs(rs []*syntax.Redirect) bool {
	for _, r := range rs {
		w := literal(r.Word)
		switch {
		case r.Op == syntax.DplOut && (w == "1" || w == "2" || w == "-"):
		case (r.Op == syntax.RdrOut || r.Op == syntax.AppOut) && w == "/dev/null":
		default:
			return false
		}
	}
	return true
}

// literal is a word's text when it is made of literals and quotes of them, ""
// otherwise.
func literal(w *syntax.Word) string {
	if w == nil {
		return ""
	}
	var b strings.Builder
	for _, p := range w.Parts {
		switch p := p.(type) {
		case *syntax.Lit:
			b.WriteString(p.Value)
		case *syntax.SglQuoted:
			b.WriteString(p.Value)
		case *syntax.DblQuoted:
			for _, q := range p.Parts {
				l, ok := q.(*syntax.Lit)
				if !ok {
					return ""
				}
				b.WriteString(l.Value)
			}
		default:
			return ""
		}
	}
	return b.String()
}

// wordsOf is the words of a simple command when each is a literal, and nil when
// any is not.
func wordsOf(c *syntax.CallExpr) []string {
	out := make([]string, 0, len(c.Args))
	for _, a := range c.Args {
		w := literal(a)
		if w == "" && len(a.Parts) > 0 {
			return nil
		}
		out = append(out, w)
	}
	return out
}

// plainStatus matches an exit status written as a number with no leading zero.
var plainStatus = regexp.MustCompile(`^(0|[1-9][0-9]*)$`)

// callEffect reads a simple command.
func callEffect(c *syntax.CallExpr, redirs []*syntax.Redirect) effect {
	if len(c.Args) == 0 {
		return green("") // assignments alone
	}
	word := literal(c.Args[0])
	if word == "" {
		return unread("a command word that is not a literal, which the check does not read")
	}
	switch {
	case cannotFailWord(word):
		if !quietRedirs(redirs) {
			return unread("a redirect to a file, which the check does not read")
		}
		return green("")
	case word == "exit":
		return exitEffect(wordsOf(c))
	case word == "set":
		return setEffect(wordsOf(c))
	}
	return failing()
}

// exitEffect reads an exit: `exit 0` ends the shell green, an exit with a
// number other than 0 ends it red whatever errexit is, and any other is not read.
func exitEffect(w []string) effect {
	switch {
	case len(w) == 2 && plainStatus.MatchString(w[1]) && w[1] == "0":
		return effect{kind: kindExitZero, form: "exit 0"}
	case len(w) == 2 && plainStatus.MatchString(w[1]):
		return effect{abort: true, exitNonZero: true, fails: true}
	}
	return unread("an exit whose status is not a plain number, which the check does not read")
}

// setEffect reads a set with one option on its own. Any other set is not read,
// and is unjudged at once when it may stop the shell or turn errexit off.
func setEffect(w []string) effect {
	if w == nil {
		// a word that is not a literal could be any option, stopping the shell included
		return effect{unknown: "a set with a word that is not a literal, which the check does not read", stops: true}
	}
	args := w[1:]
	switch strings.Join(args, " ") {
	case "-e", "-o errexit":
		return effect{kind: kindErrexitOn}
	case "+e", "+o errexit":
		return effect{kind: kindErrexitOff, form: "set +e"}
	case "-n", "-o noexec":
		return effect{kind: kindNoexec, form: "set -n"}
	case "-o pipefail":
		return effect{kind: kindPipefailOn}
	case "+o pipefail":
		return effect{kind: kindPipefailOff}
	}
	return effect{unknown: "a set the check does not read", stops: mayStop(args)}
}

// mayStop says a set the check does not read could stop the shell running what
// follows or turn errexit off, so that nothing after it can be judged: a cluster
// with n after -, or e after +, or the options noexec and errexit by name.
func mayStop(args []string) bool {
	for i, a := range args {
		if a == "--" {
			return false
		}
		switch {
		case a == "noexec" || a == "errexit":
			return true
		case len(a) > 1 && a[0] == '-' && a[1] != '-' && strings.Contains(a[1:], "n"):
			return true
		case len(a) > 1 && a[0] == '+' && strings.Contains(a[1:], "e"):
			return true
		case a == "+o" && i+1 < len(args) && args[i+1] == "errexit":
			return true
		}
	}
	return false
}

// orEffect reads `X || Y`: its status is non-zero only when X fails and Y does,
// errexit ends the shell only on Y (X is before the last command of the list),
// and an exit other than 0 inside X ends it before Y is reached.
func orEffect(c *syntax.BinaryCmd, pipefail bool) effect {
	if exitsShell(c.X) {
		return effect{abort: true, exitNonZero: true, fails: true}
	}
	x, y := classify(c.X, pipefail), classify(c.Y, pipefail)
	switch {
	case x.unknown != "":
		return x
	case y.unknown != "":
		return y
	case x.exitNonZero:
		return effect{abort: true, exitNonZero: true, fails: true}
	}
	form := ""
	switch w := firstWord(c.Y); {
	case y.kind == kindExitZero:
		form = "|| exit 0"
	case y.exitNonZero && x.fails:
		return effect{abort: true, exitNonZero: true, fails: true}
	case y.exitNonZero:
		return effect{} // X cannot fail, so the exit is never reached
	case y.kind == kindStmt && !y.fails && cannotFailWord(w):
		form = "|| " + w
	}
	return effect{abort: y.abort && y.kind == kindStmt, fails: x.fails && y.fails, form: form}
}

// andEffect reads `X && Y`: errexit ends the shell only on Y, the command after
// the final &&, so with Y a plain command the list can fail the member, and an
// exit other than 0 in it ends the shell. A list ending in a command that cannot
// fail is not read: its status is X's, which matters only for the last statement.
func andEffect(c *syntax.BinaryCmd, pipefail bool) effect {
	if exitsShell(c.X) || exitsShell(c.Y) {
		return effect{abort: true, exitNonZero: true, fails: true}
	}
	x, y := classify(c.X, pipefail), classify(c.Y, pipefail)
	switch {
	case x.unknown != "":
		return x
	case y.unknown != "":
		return y
	case y.kind == kindStmt && y.abort && y.fails:
		return failing()
	}
	return unread("an && list that ends in a command that cannot fail, which the check does not read")
}

// pipeEffect reads a pipeline: with no pipefail its status is the last command's
// and errexit ends the shell on that alone, so a failing command before cat or
// tee cannot fail the member; with pipefail any plain command that fails does.
func pipeEffect(c *syntax.BinaryCmd, pipefail bool) effect {
	stages := []*syntax.Stmt{c.Y}
	for x := c.X; ; {
		b, ok := x.Cmd.(*syntax.BinaryCmd)
		if !ok || (b.Op != syntax.Pipe && b.Op != syntax.PipeAll) {
			stages = append(stages, x)
			break
		}
		stages = append(stages, b.Y)
		x = b.X
	}
	anyFails, allGreen := false, true
	for _, s := range stages {
		e := classifyStage(s, pipefail)
		if e.unknown != "" {
			return e
		}
		anyFails = anyFails || e.fails
		allGreen = allGreen && !e.fails
	}
	last := classifyStage(stages[0], pipefail)
	switch {
	case allGreen:
		return green("")
	case pipefail:
		return failing()
	case last.fails:
		return failing()
	}
	return green("| " + firstWord(stages[0])) // only cat, tee or a command that cannot fail ends a pipeline here
}

// classifyStage is a command of a pipeline, where cat and tee read what comes
// before them and cannot fail.
func classifyStage(s *syntax.Stmt, pipefail bool) effect {
	if w := firstWord(s); w == "cat" || w == "tee" {
		return green("")
	}
	return classify(s, pipefail)
}

// firstWord is the command word of a simple command, "" for anything else.
func firstWord(s *syntax.Stmt) string {
	c, ok := s.Cmd.(*syntax.CallExpr)
	if !ok || s.Negated || len(c.Args) == 0 {
		return ""
	}
	return literal(c.Args[0])
}

// exitsShell says the statement holds, in the shell that runs it, an exit other
// than `exit 0` with a plain number: it ends the shell with that status before
// an `|| true` after it is reached. An exit inside a subshell, a command or
// process substitution, a command of a pipeline or a statement in the background
// ends only that child, and is not looked for.
func exitsShell(s *syntax.Stmt) bool {
	found := false
	syntax.Walk(s, func(n syntax.Node) bool {
		switch n := n.(type) {
		case *syntax.Subshell, *syntax.CmdSubst, *syntax.ProcSubst, *syntax.FuncDecl, *syntax.ArithmExp:
			return false
		case *syntax.Stmt:
			if n.Background {
				return false
			}
		case *syntax.BinaryCmd:
			if n.Op == syntax.Pipe || n.Op == syntax.PipeAll {
				return false
			}
		case *syntax.CallExpr:
			if w := wordsOf(n); len(w) == 2 && w[0] == "exit" && plainStatus.MatchString(w[1]) && w[1] != "0" {
				found = true
			}
		}
		return !found
	})
	return found
}

// canFail holds one member of a tier to the rule that it can fail: a member
// that cannot is a runs finding naming the Taskfile, the task and the member,
// and one that could not be read is said as not checked, never passed.
func (t *tierRun) canFail(tier Tier, c command) {
	v := CannotFail(c.text, c.pipefail)
	switch {
	case v.Why != "":
		t.add(t.in.Tiers.Rules.Runs, tier.Name+" member "+v.Why, c.line,
			"a member of "+tier.Name+" cannot fail ("+v.Why+"), so go-task ends it green whatever it runs: "+short(c.text))
	case v.Unjudged != "":
		t.res.NotChecked = append(t.res.NotChecked, NotChecked{Tool: TaskTool, Partial: true,
			Why: "a member of " + tier.Name + " was not read for a failure it cannot make (" + v.Unjudged + "): " + short(c.text)})
	}
}
