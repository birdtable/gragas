package audit

import (
	"regexp"
	"slices"
	"strings"

	"mvdan.cc/sh/v3/syntax"

	"github.com/birdtable/gragas/internal/ordered"
)

// guardForce is the guard's first test, written out: CLI_FORCE is true for --force and for --force-all alike, so
// the guard reads the same in both modes of go-task.
const guardForce = `[ "{{.CLI_FORCE}}" != true ]`

// guardTest is the guard's second test: an enabling variable that says 1.
var guardTest = regexp.MustCompile(`^\[ "\{\{\.([A-Za-z_][A-Za-z0-9_]*)\}\}" = 1 \]$`)

// statusChain is the first line of a task's status: and its statements, when that line is one statement whose
// top node is an && chain of at least two tests, read with mvdan.cc/sh, the parser go-task runs it with: no
// &, !, coprocess or redirect on it, and no ||, ; or newline at its top.
func statusChain(task *ordered.Map) (first string, ops []*syntax.Stmt, ok bool) {
	if l := task.List("status"); len(l) > 0 {
		if s, isStr := l[0].(string); isStr {
			first = s
		}
	} else {
		first = task.Str("status")
	}
	f, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(first), "")
	if err != nil || len(f.Stmts) != 1 {
		return "", nil, false
	}
	st := f.Stmts[0]
	if st.Background || st.Negated || st.Coprocess || len(st.Redirs) > 0 {
		return "", nil, false
	}
	ops = andChain(st)
	if len(ops) < 2 {
		return "", nil, false
	}
	return first, ops, true
}

// guardOf is the variable a task's status: guard names, and whether its first line is the guard: one
// statement whose top node is an && chain with the two tests of the guard leftmost. A tail inside the chain
// may stay, since the guard false keeps the whole line false; an ||, a ;, a newline or a & after it may not,
// since the line then ends green or runs on.
func guardOf(task *ordered.Map) (string, bool) {
	first, ops, ok := statusChain(task)
	if !ok {
		return "", false
	}
	src := func(s *syntax.Stmt) string { return first[s.Pos().Offset():s.End().Offset()] }
	m := guardTest.FindStringSubmatch(src(ops[1]))
	if src(ops[0]) != guardForce || m == nil {
		return "", false
	}
	return m[1], true
}

// stagedTest is the third test of the staged-inputs shape: none of the member's own INPUTS is staged.
const stagedTest = `[ -z "$(git diff --cached --name-only -- {{.INPUTS}})" ]`

// stagedShape says a task's status: is the one skip the commit tier may hold besides a declared one:
// its first line is exactly the guard (--force runs the member whole, and a variable that is 1
// only when the commit's call says so), then the staged test and nothing after it, so the skip depends only on
// whether the member's own inputs have staged changes; and the task declares those inputs itself, as its own
// vars: INPUTS, a non-empty literal or template of the file's own inputs. The variable is off in the file's own
// vars: (a literal that is not 1 or true), so no run is skipped until the commit's call turns it on. Any other
// shape, a tail such as && true, a different test, a task with no INPUTS of its own, stays a finding, and so
// does this one in any tier but the commit's (tierRun.excused reads Tier.Staged).
func (t *tierRun) stagedShape(task, file *ordered.Map) bool {
	first, ops, ok := statusChain(task)
	if !ok || len(ops) != 3 {
		return false
	}
	src := func(s *syntax.Stmt) string { return first[s.Pos().Offset():s.End().Offset()] }
	m := guardTest.FindStringSubmatch(src(ops[1]))
	if src(ops[0]) != guardForce || m == nil || src(ops[2]) != stagedTest {
		return false
	}
	own := task.Sub("vars")
	if own == nil {
		return false
	}
	if _, set := own.Get(m[1]); set {
		return false
	}
	vars := file.Sub("vars")
	if vars == nil {
		return false
	}
	if inputs, present := own.Get("INPUTS"); !present || !t.inputsPlain(asString(inputs), vars, 0) {
		return false
	}
	v, declared := vars.Get(m[1])
	s, literal := v.(string)
	return declared && literal && !strings.Contains(s, "{{") && s != "1" && s != "true"
}

// asString is v when it is a string, and "" otherwise.
func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// andChain is the statements of st when it is one pure && chain, in order, and nil when its top node is
// anything else: an ||, a ;, a newline, a & or a !. What an operand holds inside itself stays its own.
func andChain(st *syntax.Stmt) []*syntax.Stmt {
	bc, ok := st.Cmd.(*syntax.BinaryCmd)
	if !ok {
		return []*syntax.Stmt{st}
	}
	if bc.Op != syntax.AndStmt {
		return nil
	}
	l, r := andChain(bc.X), andChain(bc.Y)
	if l == nil || r == nil {
		return nil
	}
	return append(l, r...)
}

// declaredSkip says a task's status: is a declared skip: its first line is the guard above, so
// --force runs the member whole, and the variable that enables it is off until a caller passes it:
// the file's own vars: declare it as a literal string that is not 1 or true and holds no template, the
// guarded task's own vars: do not set it, and no call on the way to it passes it a value that may be 1
// (go-task ranks a call's vars: above both; a call may pass a literal string that is not 1 or true and
// holds no template, or forward the variable as {{.NAME}}, which is the file's value or one an earlier call
// passed, and that call is judged on its own). A variable that is a command (sh:), a template, absent or
// set again by the task is read from where go-task reads it, which this audit does not follow, so it
// fails closed.
func (t *tierRun) declaredSkip(task, file *ordered.Map) bool {
	name, ok := guardOf(task)
	if !ok || t.unsafe[name] > 0 || t.unsafeReached(name) {
		return false
	}
	if own := task.Sub("vars"); own != nil {
		if _, set := own.Get(name); set {
			return false
		}
	}
	vars := file.Sub("vars")
	if vars == nil {
		return false
	}
	v, declared := vars.Get(name)
	s, literal := v.(string)
	return declared && literal && !strings.Contains(s, "{{") && s != "1" && s != "true"
}

// forceHeld says a task refuses to run without go-task's --force: a precondition whose sh is the test
// that CLI_FORCE is true.
func forceHeld(task *ordered.Map) bool {
	for _, p := range task.List("preconditions") {
		if m, ok := p.(*ordered.Map); ok && m.Str("sh") == `[ "{{.CLI_FORCE}}" = true ]` {
			return true
		}
	}
	return false
}

// excused says the key k of a task, a tier's own or one a tier calls, skips nothing the tier is held
// to. Under a tier task that refuses to run without --force, a status: whose first line is the CLI_FORCE
// guard is excused: it is false in both modes of go-task. A plain status:, sources: or generates: is
// not, since with go-task's gentle force (a .taskrc.yml or a .env can turn it on) --force forces only the
// task named on the command line and every task it calls is still judged by its own checks. Otherwise
// only a declared skip is excused, and only in a tier that is not whole: the commit's and the push's may
// hold a member behind it, the release and the landing run every member.
func (t *tierRun) excused(k string, task *ordered.Map) bool {
	if k != "status" {
		return false
	}
	if t.forced {
		_, guarded := guardOf(task)
		return guarded
	}
	return !t.whole && (t.declaredSkip(task, t.doc) || (t.staged && t.stagedShape(task, t.doc)))
}

// forRuns says a call's for: runs its task, as far as the audit can read what it iterates: a literal
// list with an item, a matrix whose every axis is one, or a variable of the file's own vars: that is
// one (a matrix axis may be a ref to it) and that no task or call on the path sets again, since go-task
// resolves a for: in the scope of the task that holds it, where those values outrank the file's. An empty
// list or variable, an empty axis, sources and anything the audit cannot read runs it never, as far as
// the audit can tell, so the call skips. The callee counts once, whatever the number of items.
func (t *tierRun) forRuns(m *ordered.Map) bool {
	v, ok := m.Get("for")
	if !ok {
		return false
	}
	switch f := v.(type) {
	case []any:
		return len(f) > 0
	case *ordered.Map:
		if mx := f.Sub("matrix"); mx != nil {
			if mx.Len() == 0 {
				return false
			}
			for _, axis := range mx.Keys() {
				if !t.iterates(mx, axis) {
					return false
				}
			}
			return true
		}
		return t.listVar(f.Str("var"))
	}
	return false
}

// iterates says the value of a matrix axis is a list with an item, written out or by a ref to a variable.
func (t *tierRun) iterates(matrix *ordered.Map, axis string) bool {
	v, _ := matrix.Get(axis)
	switch a := v.(type) {
	case []any:
		return len(a) > 0
	case *ordered.Map:
		return t.listVar(strings.TrimPrefix(a.Str("ref"), "."))
	}
	return false
}

// listVar says the file's own vars: hold name as a list with an item, and nothing on the path sets it again.
func (t *tierRun) listVar(name string) bool {
	vars := t.doc.Sub("vars")
	if name == "" || vars == nil || t.shadow[name] > 0 {
		return false
	}
	return len(vars.List(name)) > 0
}

// varNames is the names a mapping's vars: sets.
func varNames(m *ordered.Map) []string {
	if vars := m.Sub("vars"); vars != nil {
		return vars.Keys()
	}
	return nil
}

// push notes that the tasks and calls on the path being read set the names again, and pop takes it back.
func (t *tierRun) push(names []string) {
	if t.shadow == nil {
		t.shadow = map[string]int{}
	}
	for _, n := range names {
		t.shadow[n]++
	}
}

func (t *tierRun) pop(names []string) {
	for _, n := range names {
		t.shadow[n]--
	}
}

// unsafeNames is the names a call's vars: sets to a value that may be 1: anything but a literal string that is
// not 1 or true and holds no template, and the forward of the name itself, {{.NAME}}.
func unsafeNames(call *ordered.Map) []string {
	vars := call.Sub("vars")
	if vars == nil {
		return nil
	}
	var out []string
	for _, name := range vars.Keys() {
		v, _ := vars.Get(name)
		s, literal := v.(string)
		forward := literal && s == "{{."+name+"}}"
		if forward || (literal && !strings.Contains(s, "{{") && s != "1" && s != "true") {
			continue
		}
		out = append(out, name)
	}
	return out
}

// pushCall and popCall note, and take back, that a call on the path passes names a value that may be 1.
// A nil call is none.
func (t *tierRun) pushCall(call *ordered.Map) {
	if call == nil {
		return
	}
	if t.unsafe == nil {
		t.unsafe = map[string]int{}
	}
	for _, n := range unsafeNames(call) {
		t.unsafe[n]++
	}
}

func (t *tierRun) popCall(call *ordered.Map) {
	if call == nil {
		return
	}
	for _, n := range unsafeNames(call) {
		t.unsafe[n]--
	}
}

// whyVar is the var a task that is Linux-only by design names its reason in.
const whyVar = "PLATFORMS_WHY"

// platformOnly says a task a tier calls is declared Linux-only by design: its own platforms:
// is a list of plain system names (no "!" entry, no non-string) that has linux in it, so it runs there and no
// system is taken out of it, and its own vars: names why, as a literal that holds a word and no template, so the
// reason is the author's and not computed. Only the platforms: of that task itself is excused (heldBy): a
// platforms: on a command inside it, or on a task it calls, stays a finding, since each task is read once and an
// excusal that followed the path would depend on which caller reaches a shared task first. The tier's own task
// is never read this way (taskLevel holds its keys), and a task that is merely platform-bound, with no reason,
// stays a finding.
func platformOnly(task *ordered.Map) bool {
	list := ordered.Strings(task.List("platforms"))
	if len(list) == 0 || len(list) != len(task.List("platforms")) || !slices.Contains(list, "linux") ||
		slices.ContainsFunc(list, func(s string) bool { return strings.HasPrefix(strings.TrimSpace(s), "!") }) {
		return false
	}
	vars := task.Sub("vars")
	if vars == nil {
		return false
	}
	why, present := vars.Get(whyVar)
	s := asString(why)
	return present && strings.TrimSpace(s) != "" && !strings.Contains(s, "{{")
}

// inputsTemplate is a reference to one of the file's own vars, as go-task writes it.
var inputsTemplate = regexp.MustCompile(`\{\{\s*\.([A-Za-z_][A-Za-z0-9_]*)\s*\}\}`)

// inputsPlain is the one thing INPUTS may hold, since go-task splices it into the shell line of the staged
// test: file names and globs, in single quotes or not, and nothing the shell reads as more (no ; & | > < $ `
// ( ) \ newline or double quote, and quotes that close). A {{.NAME}} is read through the file's own vars: the
// value is judged the same way, to a depth of a few; a name that any task or call of the file or its includes sets
// is refused whatever it holds, on the path or not (go-task ranks those above the file's, so the value spliced is not
// the one read here, and each task is read once, so the first caller would decide it for every other: setAnywhere); a reference to anything else, a var that is not a
// literal string, or any other template, is refused, since what it holds is not the author's to see here.
func (t *tierRun) inputsPlain(s string, vars *ordered.Map, depth int) bool {
	if strings.TrimSpace(s) == "" || depth > 4 {
		return false
	}
	rest := inputsTemplate.ReplaceAllStringFunc(s, func(ref string) string {
		name := inputsTemplate.FindStringSubmatch(ref)[1]
		v, ok := vars.Get(name)
		val, lit := v.(string)
		if !ok || !lit || t.setAnywhere(name) || !t.inputsPlain(val, vars, depth+1) {
			return "\x00"
		}
		return "x"
	})
	if strings.Contains(rest, "\x00") || strings.Contains(rest, "{{") || strings.Count(rest, "'")%2 != 0 {
		return false
	}
	return !strings.ContainsAny(rest, ";&|<>$`()\\\"\n\r\t")
}

// setAnywhere says some task, call or include of the files read sets the var name in its vars:, wherever it is
// called from. The path-based counts (shadow, unsafe) cannot say it for a task read once, since the first caller
// decides what a later one gets; INPUTS is spliced into the shell, so it is held to the stricter reading.
func (t *tierRun) setAnywhere(name string) bool {
	if t.set == nil {
		t.set = map[string]bool{}
		for _, sc := range t.scopes {
			collectNames(sc.doc, t.set, varNames)
		}
	}
	return t.set[name]
}

// collectNames adds to set the names, as names says them of a mapping, of a Taskfile's tasks, the calls among their
// deps and cmds, and its includes.
func collectNames(doc *ordered.Map, set map[string]bool, names func(*ordered.Map) []string) {
	if inc := doc.Sub("includes"); inc != nil {
		for _, ns := range inc.Keys() {
			addNames(inc.Sub(ns), set, names)
		}
	}
	tasks := doc.Sub("tasks")
	if tasks == nil {
		return
	}
	for _, name := range tasks.Keys() {
		collectTask(tasks.Sub(name), set, names)
	}
}

// collectTask adds the names of a task and of the calls among its deps and cmds.
func collectTask(task *ordered.Map, set map[string]bool, names func(*ordered.Map) []string) {
	addNames(task, set, names)
	if task == nil {
		return
	}
	for _, key := range []string{"deps", "cmds"} {
		for _, e := range task.List(key) {
			if m, ok := e.(*ordered.Map); ok {
				addNames(m, set, names)
				addNames(m.Sub("defer"), set, names)
			}
		}
	}
}

// addNames adds the names of m; a nil mapping has none.
func addNames(m *ordered.Map, set map[string]bool, names func(*ordered.Map) []string) {
	if m == nil {
		return
	}
	for _, k := range names(m) {
		set[k] = true
	}
}

// unsafeReached says some task or call that the tier being read reaches, in any order, sets the var name to a value
// that may be 1 (unsafeNames). A task is read once, so a plain call that reaches it first would hold the declared
// skip for a later call that passes 1, which go-task runs with the skip on. It is per tier, since the commit's
// call may turn a shared task's guard on while the push's must not.
func (t *tierRun) unsafeReached(name string) bool { return t.unsafeAll[name] }

// reachUnsafe adds to out the names that the tasks and calls reachable from the task name, in the scope prefix,
// set to a value that may be 1: each task's own vars:, and the vars: of each call among its deps and cmds.
func (t *tierRun) reachUnsafe(prefix, name string, seen map[string]bool, out map[string]bool) {
	key, task, in := t.taskOf(prefix, name)
	if task == nil || seen[key] {
		return
	}
	seen[key] = true
	for _, n := range unsafeNames(task) {
		out[n] = true
	}
	for _, list := range []string{"deps", "cmds"} {
		for _, e := range task.List(list) {
			if callee := calleeOf(e, list == "deps", out); callee != "" {
				t.reachUnsafe(in, callee, seen, out)
			}
		}
	}
}

// calleeOf is the task an entry of deps or cmds calls, "" when it calls none, and adds to out the names its call
// sets to a value that may be 1. A bare string is a call only in deps.
func calleeOf(e any, dep bool, out map[string]bool) string {
	switch c := e.(type) {
	case string:
		if dep {
			return c
		}
	case *ordered.Map:
		// A call under defer: runs its task with its own vars, as a call of the list does.
		for _, call := range []*ordered.Map{c, c.Sub("defer")} {
			if call == nil {
				continue
			}
			for _, n := range unsafeNames(call) {
				out[n] = true
			}
		}
		if d := c.Sub("defer"); d != nil {
			return d.Str("task")
		}
		return c.Str("task")
	}
	return ""
}
