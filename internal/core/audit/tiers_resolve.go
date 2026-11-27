package audit

import (
	"errors"
	"io/fs"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/birdtable/gragas/internal/digest"
	"github.com/birdtable/gragas/internal/ordered"
	"github.com/birdtable/gragas/internal/port"
)

// maxIncludeDepth is how many includes below the Taskfile are followed; an include that nests
// deeper is a finding, so a cycle ends.
const maxIncludeDepth = 8

// scope is one Taskfile the tiers read: the file itself or one it includes.
type scope struct {
	doc  *ordered.Map
	data []byte
	file string
	// parent is the scope that includes this one, and flatten, excludes say how: go-task puts a
	// flattened include's tasks in the parent, so they are not under the include's namespace, and
	// drops each task excludes: names.
	parent   string
	flatten  bool
	excludes []string
}

// loadScopes reads the Taskfile's includes, local ones from the repository and remote ones from the
// resolver the input names, into t.scopes by namespace (a:b for b included by a). It adds a finding
// for each include it cannot read and for each remote one that is not pinned.
func (t *tierRun) loadScopes() {
	t.scopes = map[string]*scope{"": {doc: t.doc, data: t.data, file: t.file}}
	t.alias = map[string]string{}
	t.includes(t.scopes[""], "", 0)
}

func (t *tierRun) includes(sc *scope, prefix string, depth int) {
	incs := sc.doc.Sub("includes")
	if incs == nil {
		return
	}
	for _, ns := range incs.Keys() {
		v, _ := incs.Get(ns)
		line := incs.KeyLine(ns)
		file, checksum, dir, optional := includeOf(v)
		if file == "" {
			t.addAt(sc.file, t.in.Tiers.Rules.Tier, "include "+prefix+ns, line, "the include "+ns+" names no taskfile, so its members are not seen")
			continue
		}
		if depth >= maxIncludeDepth {
			t.addAt(sc.file, t.in.Tiers.Rules.Tier, "include "+prefix+ns, line, "the include "+ns+" is nested more than "+strconv.Itoa(maxIncludeDepth)+" deep, so its members are not seen")
			continue
		}
		data, name, ok := t.readInclude(sc, include{ns: ns, file: file, checksum: checksum, dir: dir, optional: optional, line: line})
		if !ok {
			continue
		}
		v2, err := ordered.ParseStrict(data)
		doc, isMap := v2.(*ordered.Map)
		if err != nil || !isMap {
			t.addAt(sc.file, t.in.Tiers.Rules.Tier, "include "+prefix+ns, line, "the include "+ns+" ("+name+") cannot be read as a mapping, so its members are not seen")
			continue
		}
		flatten, excludes, aliases := includeShape(v)
		child := &scope{doc: doc, data: data, file: name, parent: strings.TrimSuffix(prefix, ":"), flatten: flatten, excludes: excludes}
		t.scopes[prefix+ns] = child
		for _, a := range aliases {
			t.alias[prefix+a] = prefix + ns
		}
		t.includes(child, prefix+ns+":", depth+1)
	}
}

// includeOf is the taskfile, the checksum, the dir and whether it is optional, of an includes value
// written as a string or as a mapping.
func includeOf(v any) (file, checksum, dir string, optional bool) {
	switch v := v.(type) {
	case string:
		return v, "", "", false
	case *ordered.Map:
		return v.Str("taskfile"), v.Str("checksum"), v.Str("dir"), v.Truthy("optional")
	}
	return "", "", "", false
}

// includeShape is how an includes value says its tasks reach the includer: flatten: true puts them
// at its root, excludes: names the ones dropped and aliases: the other namespaces the include answers to.
func includeShape(v any) (flatten bool, excludes, aliases []string) {
	m, ok := v.(*ordered.Map)
	if !ok {
		return false, nil, nil
	}
	names := func(k string) []string {
		var out []string
		for _, e := range m.List(k) {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return m.Truthy("flatten"), names("excludes"), names("aliases")
}

// remote says an include's taskfile is read from the network, as go-task reads it.
func remote(file string) bool {
	return strings.Contains(file, "://") || strings.HasPrefix(file, "git::")
}

// readInclude gives the text of one include and the name its findings go under. A remote include
// needs a checksum and, when it is a git address, a ref; one the resolver cannot give is a finding
// that names it, never a silent pass.
// include is one includes: entry as written.
type include struct {
	ns, file, checksum, dir string
	optional                bool
	line                    int
}

func (t *tierRun) readInclude(sc *scope, in include) ([]byte, string, bool) {
	ns, file, checksum, dir, optional, line := in.ns, in.file, in.checksum, in.dir, in.optional, in.line
	key := "include " + ns
	if remote(file) {
		if checksum == "" {
			t.addAt(sc.file, t.in.Tiers.Rules.Tier, key+" checksum", line, "the remote include "+ns+" has no checksum:, so the file a run reads is not the file reviewed")
		}
		if isGit(file) && !strings.Contains(file, "ref=") {
			t.addAt(sc.file, t.in.Tiers.Rules.Tier, key+" ref", line, "the git include "+ns+" has no ref=, so it reads whatever the default branch holds")
		}
		data := t.in.Remote[file]
		if len(data) == 0 {
			t.addAt(sc.file, t.in.Tiers.Rules.Tier, key+" unread", line, "the remote include "+ns+" ("+file+") cannot be read here, so its members are not seen")
			return nil, "", false
		}
		if want := strings.TrimPrefix(checksum, "sha256:"); checksum != "" && digest.Hex(data) != strings.ToLower(want) {
			t.addAt(sc.file, t.in.Tiers.Rules.Tier, key+" checksum mismatch", line, "the text of the remote include "+ns+" is not the one its checksum: names, so the file a run checks is not the file read here")
			return nil, "", false
		}
		return data, file, true
	}
	base := path.Join(path.Dir(sc.file), dir)
	var outside error // a link the repository's reader does not follow
	for _, p := range []string{path.Join(base, file), path.Join(base, file, "Taskfile.yml"), path.Join(base, file, "Taskfile.yaml")} {
		b, err := fs.ReadFile(t.in.Repo, p)
		switch {
		case err == nil:
			return b, p, true
		case outside == nil && errors.Is(err, port.ErrOutside):
			outside = err
		}
	}
	if outside != nil {
		t.addAt(sc.file, t.in.Tiers.Rules.Tier, key+" unread", line, "the include "+ns+" names "+file+", which is there and cannot be read inside the repository ("+outside.Error()+"), so its members are not seen")
		return nil, "", false
	}
	if !optional {
		t.addAt(sc.file, t.in.Tiers.Rules.Tier, key+" unread", line, "the include "+ns+" names "+file+", which is not in the repository, so its members are not seen")
	}
	return nil, "", false
}

// isGit says an include's taskfile is read from a git repository, as go-task reads it: a git address
// (git::, git@) or an https one whose path runs through a .git folder to the file.
func isGit(file string) bool {
	return strings.HasPrefix(file, "git") || strings.Contains(file, ".git//")
}

// taskIn is the task of a scope by its name there, and the scope whose file holds it: its own tasks
// that the includer's excludes: leaves, then those of an include that flattens into it. Nil when the
// scope or the task is not there.
func (t *tierRun) taskIn(scopeName, name string) (*ordered.Map, string) {
	sc := t.scopes[scopeName]
	if sc == nil || slices.Contains(sc.excludes, name) {
		return nil, ""
	}
	if tasks := sc.doc.Sub("tasks"); tasks != nil {
		if m := tasks.Sub(name); m != nil {
			return m, scopeName
		}
	}
	names := make([]string, 0, len(t.scopes))
	for ns := range t.scopes {
		names = append(names, ns)
	}
	slices.Sort(names)
	for _, ns := range names {
		if c := t.scopes[ns]; c.flatten && c.parent == scopeName && ns != scopeName {
			if m, in := t.taskIn(ns, name); m != nil {
				return m, in
			}
		}
	}
	return nil, ""
}

// taskOf finds a task by the name written in the scope prefix: its own file's task first, then the
// name from the root (go-task's leading colon is dropped).
func (t *tierRun) taskOf(prefix, name string) (key string, task *ordered.Map, in string) {
	for _, cand := range []string{prefix + name, strings.TrimPrefix(name, ":")} {
		if m, in := t.taskIn("", cand); m != nil {
			return cand, m, in
		}
		if i := strings.LastIndex(cand, ":"); i > 0 {
			ns := cand[:i]
			if real, ok := t.alias[ns]; ok {
				ns = real
			}
			// A flattened include's tasks are in its includer, not under its namespace.
			if sc := t.scopes[ns]; sc != nil && !sc.flatten {
				if m, in := t.taskIn(ns, cand[i+1:]); m != nil {
					return ns + ":" + cand[i+1:], m, in
				}
			}
		}
	}
	return "", nil, ""
}

// members is the commands a task runs, deps first and then its cmds in order, a call of another task
// (a dep or a cmds entry with task:) replaced by that task's own, and the task of an included file
// read from its file. top says the task is the tier itself, whose own keys taskLevel holds; a task it
// calls is held to the same keys (heldBy), and the keys of the call itself are held to callKeys. Each task is read once.
func (t *tierRun) members(prefix, name string, task *ordered.Map, top bool, callLine int, seen map[string]bool) []command {
	sc := t.scopes[prefix]
	if sc == nil {
		return nil
	}
	own := varNames(task)
	t.push(own)
	defer t.pop(own)
	// The task's own vars: that may be 1 reach what it forwards as {{.NAME}}, which go-task resolves in its scope.
	t.pushCall(task)
	defer t.popCall(task)
	var out []command
	at := func(kind string, i int) int {
		if top {
			return ordered.LineAt(sc.doc, []string{"tasks", name, kind, strconv.Itoa(i)})
		}
		return callLine
	}
	for i, v := range task.List("deps") {
		callee, skip := "", ""
		var passed []string
		var entry *ordered.Map
		switch d := v.(type) {
		case string:
			callee = d
		case *ordered.Map:
			entry = d
			callee, skip, passed = d.Str("task"), t.callKeySkip(d), varNames(d)
		}
		t.push(passed)
		t.pushCall(entry)
		out = append(out, markAll(t.call(prefix, callee, at("deps", i), seen), skip)...)
		t.popCall(entry)
		t.pop(passed)
	}
	// pipefail is the root file's set: or the task's own, which reaches the commands it holds and not
	// the tasks it calls (each of those has its own). go-task joins the root's set: to every task and
	// drops the set: of an included file when it merges it in.
	pipefail := setsPipefail(t.scopes[""].doc) || setsPipefail(task)
	for i, v := range task.List("cmds") {
		out = append(out, t.cmdOf(prefix, v, at("cmds", i), cmdCtx{top: top, pipefail: pipefail}, seen)...)
	}
	if !top {
		out = t.heldBy(task, name, out)
	}
	return out
}

// cmdCtx is what a task says of the commands it holds: whether it is the tier itself, and whether pipefail is
// on for them.
type cmdCtx struct{ top, pipefail bool }

// cmdOf is the commands one entry of cmds: gives: a string, a mapping with cmd:, or a call. A command
// has a timeout only when its own mapping says one: go-task 3.54.0 reads a timeout on a command and on a
// dep, and ignores a task's own timeout: without a word.
func (t *tierRun) cmdOf(prefix string, v any, line int, h cmdCtx, seen map[string]bool) []command {
	top, pipefail := h.top, h.pipefail
	switch m := v.(type) {
	case string:
		if top {
			line = lineOfText(t.scopes[prefix].data, line, m)
		}
		return []command{{text: m, line: line, system: systemOf(m), pipefail: pipefail}}
	case *ordered.Map:
		text := m.Str("cmd")
		if text == "" {
			skip, passed := t.callKeySkip(m), varNames(m)
			t.push(passed)
			t.pushCall(m)
			defer t.pop(passed)
			defer t.popCall(m)
			return timedAll(markAll(t.call(prefix, m.Str("task"), line, seen), skip), m.Str("timeout") != "")
		}
		_, hasTimeout := m.Get("timeout")
		own := command{text: text, line: line, timeout: hasTimeout && m.Str("timeout") != "", system: systemOf(text), skip: skipOf(m),
			pipefail: pipefail || setsPipefail(m)}
		out := []command{own}
		if callee := budgeted(text); callee != "" && own.skip == "" {
			// A tier's budget runs the tier's members as one command (hatch budget ... -- task <members>); hatch ends red when
			// the command does, so the members are the tier's, held to the command's own timeout.
			out = append(out, timedAll(t.call(prefix, callee, line, seen), own.timeout)...)
		}
		return out
	}
	return nil
}

// budgetedCall is the task a command hands to the budget: the word after "-- {{.TASK_EXE}}".
var budgetedCall = regexp.MustCompile(`\s--\s+\{\{\s*\.TASK_EXE\s*\}\}\s+([A-Za-z0-9:_-]+)`)

// budgeted is the task a tier's budget command runs, "" when the command is no budget's.
func budgeted(text string) string {
	if m := budgetedCall.FindStringSubmatch(text); m != nil && strings.Contains(strings.ToLower(text), "budget") {
		return m[1]
	}
	return ""
}

// setsPipefail says a mapping's set: turns the shell option pipefail on.
func setsPipefail(m *ordered.Map) bool {
	v, ok := m.Get("set")
	return ok && pipefailOn(v)
}

// callKeys are the keys a call (a dep or a cmds entry with task:) may carry and still run its task on
// every host: any other key (if, platforms, for, ignore_error, defer) can stop the call or swallow its
// failure, so the callee's commands count for nothing. timeout is one: it can only end the call red, and
// go-task 3.54.0 reads it on a call as on a command, which is where a limit of a task has to go;
// the callee's commands then count as limited (timedAll).
func callKeys() []string { return []string{"task", "vars", "silent", "timeout"} }

// timedAll marks the commands of a call that carries a timeout as limited: go-task ends the call, and so each
// command in it, at the limit.
func timedAll(cmds []command, timed bool) []command {
	for i := range cmds {
		if timed {
			cmds[i].timeout = true
		}
	}
	return cmds
}

// callKeySkip is why go-task would not run a call on every host, or "" when the call carries only
// callKeys.
func (t *tierRun) callKeySkip(m *ordered.Map) string {
	passed := varNames(m)
	t.push(passed)
	defer t.pop(passed)
	for _, k := range m.Keys() {
		if k == "for" && t.forRuns(m) {
			continue
		}
		if !slices.Contains(callKeys(), k) {
			return k + ": on the call of " + m.Str("task")
		}
	}
	return ""
}

// markAll marks the commands that carry no skip yet with one.
func markAll(cmds []command, skip string) []command {
	for i := range cmds {
		if skip != "" && cmds[i].skip == "" {
			cmds[i].skip = skip
		}
	}
	return cmds
}

// heldBy marks the commands of a called task that one of its own keys (the keys a tier is held to) or
// set: noexec skips: a callee with status: is up to date on every host, one with platforms: runs on
// some only, one with sources: is skipped on a checksum of what its author listed.
func (t *tierRun) heldBy(task *ordered.Map, name string, cmds []command) []command {
	for _, k := range taskKeys() {
		if k == "platforms" && platformOnly(task) && t.linuxOnlyHeld(cmds) {
			continue
		}
		if v, ok := task.Get(k); ok && (k != "ignore_error" || truthy(v)) && !t.excused(k, task) {
			return markAll(cmds, k+": at the task "+name)
		}
	}
	if v, ok := task.Get("set"); ok && noexec(v) {
		return markAll(cmds, "set: noexec at the task "+name)
	}
	return cmds
}

// call is the commands of the task a dep or a cmds entry names, none when it names none or one that
// is not there (go-task refuses that itself) or one already read.
func (t *tierRun) call(prefix, name string, line int, seen map[string]bool) []command {
	if name == "" {
		return nil
	}
	key, task, in := t.taskOf(prefix, name)
	if task == nil || seen[key] {
		return nil
	}
	seen[key] = true
	return t.members(in, taskLocal(key, in), task, false, line, seen)
}

// taskLocal is the task's name inside its own file: key without its namespace.
func taskLocal(key, in string) string {
	if in == "" {
		return key
	}
	return strings.TrimPrefix(key, in+":")
}

func truthy(v any) bool {
	switch v := v.(type) {
	case bool:
		return v
	case string:
		return v == "true"
	}
	return false
}

// pool holds a tier's parallel members to what gates.command.12 asks of a pool: no member stops
// another at a red, so every result is collected. go-task does that by default; failfast turns it off.
func (t *tierRun) pool(tier Tier, task *ordered.Map, cmds []command) {
	if v, ok := t.doc.Get("failfast"); ok && truthy(v) {
		t.add(t.in.Tiers.Rules.Order, tier.Name+" failfast", t.doc.KeyLine("failfast"), "the file has failfast:, so a red member stops the others and a tier stops at the first red")
	}
	if v, ok := task.Get("failfast"); ok && truthy(v) {
		t.add(t.in.Tiers.Rules.Order, tier.Name+" failfast", task.KeyLine("failfast"), "the task "+tier.Name+" has failfast:, so a red member stops the others and a tier stops at the first red")
	}
	for _, c := range cmds {
		if strings.Contains(c.text, "--failfast") {
			t.add(t.in.Tiers.Rules.Order, tier.Name+" failfast", c.line, "a member of "+tier.Name+" runs go-task with --failfast, so a red member stops the others: "+short(c.text))
		}
	}
}

// linuxOnlyHeld says every member of the tier that the commands of a declared Linux-only task carry is one
// the tier table lists as Linux-only (Member.LinuxOnly): a task that holds go vet, or any member the table
// does not list, cannot be excused by saying why.
func (t *tierRun) linuxOnlyHeld(cmds []command) bool {
	for _, m := range t.tierMembers {
		if m.LinuxOnly {
			continue
		}
		if slices.ContainsFunc(cmds, func(c command) bool { return carries(c.text, m) }) {
			return false
		}
	}
	return true
}
