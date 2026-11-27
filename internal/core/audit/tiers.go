package audit

import (
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/birdtable/gragas/internal/ordered"
)

// TaskTool is the tool of the Sources row that says which Taskfile the tier
// contract reads: the same row, and the same find, the config checker reads a
// task's names through.
const TaskTool = "task"

// Member is one thing a tier must hold, as the tier table says it. A command
// of the tier holds it when it carries every word of All and, if Any is not
// empty, one of Any. Tree says the command starts a process tree and so needs
// a timeout. PerSystem says one command is asked for each declared system, and
// Flag a word the command must carry.
type Member struct {
	ID        string
	Rule      string
	All, Any  []string
	Tree      bool
	PerSystem bool
	Flag      string
	FlagRule  string
	// LinuxOnly says a task that holds this member may be declared Linux-only by design (platformOnly), so its
	// platforms: is no skip. A task that holds a member without it stays a finding.
	LinuxOnly bool
}

// Tier is one of the four tasks a repository's runner must have.
type Tier struct {
	Name    string
	Members []Member
	// Whole says the tier runs every member whole: a declared skip is no excuse in it.
	Whole bool
	// Staged says the tier is the commit tier, whose members may carry the staged-inputs shape of skip
	// (stagedShape): skipped when none of the member's own inputs is staged. No other tier may.
	Staged bool
}

// Tiers is the tier table: the tiers by name and what each holds, the files a
// pre-push hook may be in, and the rule each kind of finding cites. It is data
// (internal/host/assets/audit/tiers.yaml); this package names no tool in it.
type Tiers struct {
	Tiers []Tier
	// Hook is the file names a pre-push hook may have, the first that exists
	// is read; HookCalls is the words its text must carry.
	Hook      []string
	HookCalls []string
	// Empty is the words of a member that makes a run with no tests red.
	Empty []string
	// Land is the land job's task and the tiers it calls, in order.
	Land  Land
	Rules TierRules
	// Known is the GOOS/GOARCH pairs a declared system may be.
	Known []string
}

// Land is the land job as the tier table names it: the task the lander runs on the merged tree
// before main, and the tiers it must call, in that order. The hook's tier is one of them, so the hook's list is a
// declared subset of the land job's, and every member of every tier in Calls runs before main. An empty Task
// asks nothing.
type Land struct {
	Task  string
	Calls []string
}

// TierRules is the rule each kind of tier finding cites.
type TierRules struct {
	Tier, Order, Timeout, Background, Parallel, Hook, HookCalls, Empty, Systems, Runs, Land string
}

// TierInput is what one read of the tier contract needs: the repository, the
// Sources row of the task runner, the table, and the systems the repository
// declares (each "os" or "os/arch").
type TierInput struct {
	Repo        fs.FS
	Sources     []Source
	Tiers       Tiers
	SystemsFile string
	Systems     []Declared
	Exempt      func(rule string) (until string, ok bool)
	// Remote is the text of each remote include by its address (read from go-task's cache or the
	// pinned module); an address it lacks leaves the include unread, a finding that names it.
	// go-task itself checks the checksum: when it fetches the file.
	Remote map[string][]byte
}

// Declared is one system a repository declares, and the line of its file it is on.
type Declared struct {
	Name string
	Line int
}

// CheckTiers holds the repository's Taskfile to the tier table. It reads the
// file and the hook and runs nothing. Its findings are in the order of the
// table; a repository with no Taskfile gets the one finding that says so.
func CheckTiers(in TierInput) Result {
	res := Result{Findings: []Finding{}, Exempt: []Exempt{}, NotChecked: []NotChecked{}}
	var src *Source
	for i := range in.Sources {
		if in.Sources[i].Tool == TaskTool {
			src = &in.Sources[i]
		}
	}
	if src == nil || len(in.Tiers.Tiers) == 0 {
		return res
	}
	c := find(in.Repo, *src)
	t := &tierRun{in: in, file: c.File, data: c.Data, res: &res}
	switch {
	case !c.Present:
		t.add(in.Tiers.Rules.Tier, "tiers", 0, "there is no "+c.File+", so there are no tiers: "+strings.Join(tierNames(in.Tiers), ", "))
		return res
	default:
		v, err := ordered.ParseStrict(c.Data)
		doc, ok := v.(*ordered.Map)
		if err != nil || !ok {
			res.NotChecked = append(res.NotChecked, NotChecked{Tool: TaskTool, Why: fmt.Sprintf("%s cannot be read as a mapping: %v", c.File, err)})
			return res
		}
		t.doc = doc
		if v, ok := doc.Get("set"); ok && noexec(v) {
			t.fileNoexec = true
			t.add(in.Tiers.Rules.Runs, "set", doc.KeyLine("set"), "the file has set: noexec, so go-task's shell reads each member and runs none")
		}
		t.loadScopes()
		for _, tier := range in.Tiers.Tiers {
			t.tier(tier)
		}
		t.land()
	}
	t.declared()
	t.hook()
	return res
}

// land holds the land job's task to the table: it is there, nothing on it skips it, and its cmds
// call each tier of Land.Calls, in that order, as a call go-task runs on every host whose failure fails the land
// job. A tier the land job runs only through deps, or a call with vars, ignore_error, platforms or if, does not
// count.
func (t *tierRun) land() {
	l := t.in.Tiers.Land
	if l.Task == "" {
		return
	}
	rule, key := t.in.Tiers.Rules.Land, "land "+l.Task
	what := "the land job, which runs " + strings.Join(l.Calls, " then ") + " on the merged tree before main"
	tasks := t.doc.Sub("tasks")
	var task *ordered.Map
	if tasks != nil {
		task = tasks.Sub(l.Task)
	}
	if task == nil {
		line := t.doc.Line
		if tasks != nil {
			line = t.doc.KeyLine("tasks")
		}
		t.add(rule, key, line, "no task "+l.Task+": "+what)
		return
	}
	for _, k := range taskKeys() {
		if _, ok := task.Get(k); ok {
			t.add(rule, key+" "+k, task.KeyLine(k), "the task "+l.Task+" has "+k+":, so go-task skips it, runs it on some hosts only or lets it end green without running "+strings.Join(l.Calls, " and "))
		}
	}
	cmds := task.List("cmds")
	last, lastName := -1, ""
	for _, want := range l.Calls {
		at := -1
		for i, c := range cmds {
			if m, ok := c.(*ordered.Map); ok && m.Str("task") == want && onEveryHost(m) {
				at = i
				break
			}
		}
		switch {
		case at < 0:
			t.add(rule, key+" "+want, anchor(task), "the task "+l.Task+" does not call "+want+" as one of its cmds, with no key beyond task, silent and timeout (no vars, which can turn a member's skip on), so "+what+" is not true")
		case at < last:
			t.add(rule, key+" "+want, anchor(task), "the task "+l.Task+" calls "+want+" before "+lastName+": "+what)
		default:
			last, lastName = at, want
		}
	}
}

// onEveryHost says a call of the land task carries no key beyond task, silent and timeout, so go-task runs it on
// every host, its failure fails the land job, and it runs the tier as the table holds it. vars is not one of
// them: a call's vars reach every member, and one a member's skip guard reads
// (SKIP_UNCHANGED) turns the skip on, so the land job would run less than the tier.
func onEveryHost(m *ordered.Map) bool {
	for _, k := range m.Keys() {
		if !slices.Contains(landCallKeys(), k) {
			return false
		}
	}
	return true
}

// landCallKeys is the keys a call of the land task may carry: callKeys without vars.
func landCallKeys() []string { return []string{"task", "silent", "timeout"} }

func tierNames(t Tiers) []string {
	names := make([]string, len(t.Tiers))
	for i, tier := range t.Tiers {
		names[i] = tier.Name
	}
	return names
}

// tierRun is one read: the table, the document and where findings go.
type tierRun struct {
	in   TierInput
	file string
	data []byte
	doc  *ordered.Map
	res  *Result
	// timeoutSaid is set once the not-checked line about timeouts is made.
	timeoutSaid bool
	// fileNoexec is set when the file's own set: is noexec.
	fileNoexec bool
	// scopes are the Taskfile and the files it includes, by namespace ("" is the Taskfile).
	scopes map[string]*scope
	// alias maps a namespace an include's aliases: gives to the namespace it names, each as its scope name.
	alias map[string]string
	// whole and forced are set while a tier is read: the tier runs its members whole, and its task
	// refuses to run without --force, which ignores every status:.
	whole, forced, staged bool
	// tierMembers are the members of the tier being read.
	tierMembers []Member
	// shadow counts, for each variable name, the tasks and calls on the path being read that set it again:
	// go-task resolves a for: in the scope of the task that holds it, where those values outrank the file's.
	shadow map[string]int
	// unsafe counts, for each variable name, the calls on the path being read that pass it a value that
	// may be 1: go-task ranks a call's vars: above the file's and the task's, so a guard that reads it is
	// no longer off by default.
	unsafe map[string]int
	// set is every var name that a task's vars:, a call's vars: or an include's vars: sets anywhere in the files read; read once.
	set map[string]bool
	// unsafeAll is every var name that any task or call the tier being read reaches sets to a value that may be 1 (unsafeNames).
	unsafeAll map[string]bool
}

// add records a finding, or an exemption when the rule has a live row.
func (t *tierRun) add(rule, key string, line int, missing string) {
	f := Finding{Rule: rule, Tool: TaskTool, Key: key, File: t.file, Line: line, Missing: missing}
	if t.in.Exempt != nil {
		if until, ok := t.in.Exempt(rule); ok {
			t.res.Exempt = append(t.res.Exempt, Exempt{f, until})
			return
		}
	}
	t.res.Findings = append(t.res.Findings, f)
}

// command is one member of a tier's cmds: its text, its line, whether it
// carries a timeout, and the system its own words name. Skip is why go-task
// would not run it on every host, or would not let its failure fail the tier;
// a command with a Skip holds nothing.
type command struct {
	text    string
	line    int
	timeout bool
	system  string
	skip    string
	// quiet says the skip is a task's or the file's, said once at its own line.
	quiet bool
	// pipefail says a set: of the member, its task or the file turns the shell
	// option pipefail on, so a pipeline fails when any command of it does.
	pipefail bool
}

// setNames are the shell options a set: value names, from a string or a list.
func setNames(v any) []string {
	var names []string
	switch x := v.(type) {
	case string:
		names = []string{x}
	case []any:
		for _, e := range x {
			if s, ok := e.(string); ok {
				names = append(names, s)
			}
		}
	}
	return names
}

// noexec says a set: value turns go-task's shell into one that reads each
// command and runs none (the shell options n and noexec).
func noexec(v any) bool {
	return slices.ContainsFunc(setNames(v), func(n string) bool { return n == "n" || n == "noexec" })
}

// pipefailOn says a set: value turns the shell option pipefail on.
func pipefailOn(v any) bool { return slices.Contains(setNames(v), "pipefail") }

// memberKeys are the keys of a command mapping go-task reads and that leave
// the command run on every host with its failure failing the task. Any other
// key skips the command, swallows its failure, or is dropped without a word
// (a cmd-level env is one), so a command that has one is not counted.
func memberKeys() []string { return []string{"cmd", "timeout", "silent", "set", "shopt", "vars"} }

// taskKeys are the keys of a tier task that skip it, run it only on some
// hosts, or let it end green without running.
func taskKeys() []string {
	return []string{"ignore_error", "platforms", "if", "status", "sources", "generates"}
}

// tier holds one tier task to the table.
func (t *tierRun) tier(tier Tier) {
	tasks := t.doc.Sub("tasks")
	var task *ordered.Map
	if tasks != nil {
		task = tasks.Sub(tier.Name)
	}
	if task == nil {
		line := t.doc.Line
		if tasks != nil {
			line = t.doc.KeyLine("tasks")
		}
		t.add(t.in.Tiers.Rules.Tier, tier.Name, line, "no task "+tier.Name)
		return
	}
	t.whole, t.forced, t.staged = tier.Whole, tier.Whole && forceHeld(task), tier.Staged
	t.unsafeAll = map[string]bool{}
	t.reachUnsafe("", tier.Name, map[string]bool{}, t.unsafeAll)
	t.tierMembers = tier.Members
	cmds := t.taskLevel(tier, task)
	t.pool(tier, task, cmds)
	for _, c := range cmds {
		if c.skip != "" && c.quiet {
			continue
		}
		if c.skip != "" {
			t.add(t.in.Tiers.Rules.Runs, tier.Name+" member "+c.skip, c.line,
				"a member of "+tier.Name+" is not run by go-task on every host, or its failure does not fail the tier ("+c.skip+"): "+short(c.text))
			continue
		}
		if hasBackground(c.text) {
			t.add(t.in.Tiers.Rules.Background, tier.Name+" &", c.line, "a member of "+tier.Name+" runs a command in the background with &: "+short(c.text))
		}
		t.canFail(tier, c)
	}
	for _, m := range tier.Members {
		t.member(tier, m, cmds, task)
	}
	t.empty(tier, cmds, task)
}

// taskLevel reads a tier task's own keys, says each that skips it, and gives
// its members, none of which holds anything when the task or the file is noexec.
func (t *tierRun) taskLevel(tier Tier, task *ordered.Map) []command {
	for _, k := range taskKeys() {
		if _, ok := task.Get(k); ok && !t.excused(k, task) {
			t.add(t.in.Tiers.Rules.Runs, tier.Name+" "+k, task.KeyLine(k),
				"the task "+tier.Name+" has "+k+":, so go-task skips it, runs it on some hosts only or lets it end green without running its members")
		}
	}
	cmds := t.members("", tier.Name, task, true, 0, map[string]bool{tier.Name: true})
	level := ""
	if v, ok := task.Get("set"); ok && noexec(v) {
		level = "the task " + tier.Name
		t.add(t.in.Tiers.Rules.Runs, tier.Name+" set", task.KeyLine("set"), level+" has set: noexec, so go-task's shell reads each member and runs none")
	} else if t.fileNoexec {
		level = "the file"
	}
	for i := range cmds {
		if level != "" && cmds[i].skip == "" {
			cmds[i].skip, cmds[i].quiet = "set: noexec at "+level, true
		}
	}
	return cmds
}

// skipOf is why go-task would not run a command mapping on every host or let
// its failure fail the tier: a key beyond the ones it reads, or set: noexec.
func skipOf(m *ordered.Map) string {
	for _, k := range m.Keys() {
		switch {
		case k == "task":
			return "task: beside cmd:"
		case !slices.Contains(memberKeys(), k):
			return k + ":"
		}
	}
	if v, ok := m.Get("set"); ok && noexec(v) {
		return "set: noexec"
	}
	return ""
}

// systemOf is the "os" or "os/arch" a command names in the GOOS and GOARCH
// words written before it; "" when it names none. A cmd-level env mapping is
// not read: go-task drops it.
func systemOf(text string) string {
	goos, goarch := wordValue(text, "GOOS"), wordValue(text, "GOARCH")
	switch {
	case goos != "" && goarch != "":
		return goos + "/" + goarch
	case goos != "":
		return goos
	}
	return ""
}

// wordValue is v of the first KEY=v word of text, "" when there is none.
func wordValue(text, key string) string {
	for w := range strings.FieldsSeq(text) {
		if rest, ok := strings.CutPrefix(w, key+"="); ok {
			return strings.Trim(rest, `"'`)
		}
	}
	return ""
}

var background = regexp.MustCompile(`(^|[^&>])&($|[^&>])`)

// hasBackground says a command ends or splits on a single &.
func hasBackground(text string) bool { return background.MatchString(text) }

func short(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 60 {
		return s[:57] + "..."
	}
	return s
}

// holds says a command carries every word of All and one of Any.
func holds(c command, m Member) bool {
	return c.skip == "" && carries(c.text, m)
}

// exeExt is go-task's program suffix at a word's end: ".exe" on Windows, nothing elsewhere, so a pinned tool's path
// names the same program on every host. A member word is read across it; written inside a word, or with
// a trim marker that would join two, it stays, and the word does not match.
var exeExt = regexp.MustCompile(`\{\{\s*exeExt\s*\}\}([\s'"]|$)`)

// carries says a command text carries every word of All and one of Any, skipped or not; {{exeExt}} at a word's end
// is read as the nothing it is off Windows.
func carries(text string, m Member) bool {
	text = exeExt.ReplaceAllString(text, "$1")
	for _, w := range m.All {
		if !strings.Contains(text, w) {
			return false
		}
	}
	if len(m.Any) == 0 {
		return len(m.All) > 0
	}
	return slices.ContainsFunc(m.Any, func(w string) bool { return strings.Contains(text, w) })
}

// member holds one table row to a tier: present, with a timeout when it
// starts a process tree, with its flag, and once per declared system.
func (t *tierRun) member(tier Tier, m Member, cmds []command, task *ordered.Map) {
	var have []command
	for _, c := range cmds {
		if holds(c, m) {
			have = append(have, c)
		}
	}
	if len(have) == 0 {
		t.add(m.Rule, tier.Name+" "+m.ID, anchor(task), "the task "+tier.Name+" has no "+m.ID+" member")
		return
	}
	for _, c := range have {
		if m.Tree && c.timeout && !t.timeoutSaid {
			t.timeoutSaid = true
			t.res.NotChecked = append(t.res.NotChecked, NotChecked{Tool: TaskTool, Partial: true,
				Why: "a member's timeout holds only from go-task 3.53.1; this check cannot see the go-task version"})
		}
		if m.Tree && !c.timeout {
			t.add(t.in.Tiers.Rules.Timeout, tier.Name+" "+m.ID+" timeout", c.line, "the "+m.ID+" member of "+tier.Name+" starts a process tree and has no timeout:")
		}
		if m.Flag != "" && !strings.Contains(c.text, m.Flag) {
			t.add(m.FlagRule, tier.Name+" "+m.ID+" "+m.Flag, c.line, "the "+m.ID+" member of "+tier.Name+" runs without "+m.Flag)
		}
	}
	if m.PerSystem {
		t.systems(tier, m, have, task)
	}
}

// systems asks one member per declared system. With none declared, one member
// is enough and names no system.
func (t *tierRun) systems(tier Tier, m Member, have []command, task *ordered.Map) {
	for _, d := range t.in.Systems {
		s := d.Name
		if !slices.Contains(t.in.Tiers.Known, s) {
			continue
		}
		if !slices.ContainsFunc(have, func(c command) bool { return covers(c.system, s) }) {
			t.add(m.Rule, tier.Name+" "+m.ID+" "+s, anchor(task), "the task "+tier.Name+" has no "+m.ID+" member for "+s)
		}
	}
}

// covers says a command's system is the declared one: the same os, and the
// same arch when both name one.
func covers(have, declared string) bool {
	ho, ha, _ := strings.Cut(have, "/")
	do, da, _ := strings.Cut(declared, "/")
	return ho != "" && ho == do && (ha == "" || da == "" || ha == da)
}

// empty holds a tier that runs tests to a member that makes a run with no
// tests red: gotestsum itself exits 0 on a package with none. The member names
// gotestsum in All or among its Any (hatch tests is the other), and the rule asks only of a command that holds it through gotestsum:
// hatch tests judges an empty run itself.
func (t *tierRun) empty(tier Tier, cmds []command, task *ordered.Map) {
	tests := false
	for _, m := range tier.Members {
		if !strings.Contains(strings.Join(slices.Concat(m.All, m.Any), " "), "gotestsum") {
			continue
		}
		if slices.ContainsFunc(cmds, func(c command) bool { return holds(c, m) && strings.Contains(c.text, "gotestsum") }) {
			tests = true
		}
	}
	if !tests || len(t.in.Tiers.Empty) == 0 {
		return
	}
	if !slices.ContainsFunc(cmds, func(c command) bool {
		return slices.ContainsFunc(t.in.Tiers.Empty, func(w string) bool { return strings.Contains(c.text, w) })
	}) {
		t.add(t.in.Tiers.Rules.Empty, tier.Name+" empty", anchor(task),
			"the task "+tier.Name+" runs tests through gotestsum but no member reads its count line ("+strings.Join(t.in.Tiers.Empty, " or ")+") and exits 1, so a package with no tests is green")
	}
}

// hook holds the pre-push hook file: it is there and it calls the pre-push
// task.
func (t *tierRun) hook() {
	if len(t.in.Tiers.Hook) == 0 {
		return
	}
	for _, name := range t.in.Tiers.Hook {
		b, err := fs.ReadFile(t.in.Repo, name)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			continue
		case err != nil: // there, and unread: a link is not followed
			t.addAt(name, t.in.Tiers.Rules.Hook, "pre-push hook", 0, "the hook "+name+" is there and cannot be read inside the repository ("+err.Error()+"), so what it calls is not checked")
			return
		}
		for _, w := range t.in.Tiers.HookCalls {
			if !strings.Contains(string(b), w) {
				t.addAt(name, t.in.Tiers.Rules.HookCalls, "pre-push hook", 0, "the hook "+name+" does not call the pre-push task: it lacks "+strconv.Quote(w))
				return
			}
		}
		return
	}
	t.addAt(t.in.Tiers.Hook[0], t.in.Tiers.Rules.Hook, "pre-push hook", 0, "there is no pre-push hook file ("+strings.Join(t.in.Tiers.Hook, ", ")+")")
}

// addAt is add with the file named, for a finding that is not in the Taskfile.
func (t *tierRun) addAt(file, rule, key string, line int, missing string) {
	was := t.file
	t.file = file
	t.add(rule, key, line, missing)
	t.file = was
}

// declared holds the declared systems to the table of known pairs: an entry
// that is no GOOS/GOARCH pair is a finding at its line of the file, and none
// declared is said as a line, never left silent.
func (t *tierRun) declared() {
	if len(t.in.Systems) == 0 {
		t.res.NotChecked = append(t.res.NotChecked, NotChecked{Tool: "systems", Partial: true,
			Why: "none declared in " + t.in.SystemsFile + ", so one build and one lint member are asked for"})
		return
	}
	for _, d := range t.in.Systems {
		if !slices.Contains(t.in.Tiers.Known, d.Name) {
			t.addAt(t.in.SystemsFile, t.in.Tiers.Rules.Systems, "systems "+d.Name, d.Line, d.Name+" is not a GOOS/GOARCH pair Go builds for")
		}
	}
}

// lineOfText is the first line after from that carries text, for a member
// written as a bare string, which the YAML reader gives no line of its own. It
// falls back to from.
func lineOfText(data []byte, from int, text string) int {
	for i, l := range strings.Split(string(data), "\n") {
		if i+1 > from && strings.Contains(l, text) {
			return i + 1
		}
	}
	return from
}

// anchor is the line a finding about a tier's members goes to: its cmds, else its deps, else the task.
func anchor(task *ordered.Map) int {
	for _, k := range []string{"cmds", "deps"} {
		if l := task.KeyLine(k); l > 0 {
			return l
		}
	}
	return task.Line
}
