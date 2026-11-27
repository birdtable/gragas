package internal

import (
	"maps"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/dev/layers"
	"github.com/birdtable/gragas/internal/ordered"
)

// The Go preset's gomodguard_v2 and gomoddirectives rows are held to the
// register of libraries (supply.intake.6; gragas#951): every module a chosen
// row of the register replaces and bans, where the row names gomodguard_v2 as
// its linter, is a blocked row of the preset with the register's reason and
// the row's own module as its recommendation, and the preset blocks nothing the
// register does not list. Its depguard and forbidigo rows are held the same way
// (gragas#1017): each chosen banned row that names depguard is the preset's
// depguard rule lib-<row>, denying each import the row denies, and each call a
// chosen row forbids where it names forbidigo is a forbidigo entry, each with
// the register's why and the row's library, and the preset carries no such
// rule or entry the register does not. The register is read through #932's
// loader.

// theRegister reads the register of the tree through its schema and its loader.
func theRegister(t *testing.T) *layers.Register {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, layers.RegisterFile))
	if err != nil {
		t.Fatal(err)
	}
	// The schema is held by cmd/gragas-dev; the loader holds the rest.
	r, err := layers.ParseRegister(data, func(any) []string { return nil })
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// blockedRow is one entry of the preset's gomodguard_v2 blocked list.
type blockedRow struct {
	Module, Reason string
	Recommends     []string
	Line           int
}

// presetBlocked is the blocked list of the preset's document.
func presetBlocked(t *testing.T, doc *ordered.Map) []blockedRow {
	t.Helper()
	settings := doc.Sub("linters").Sub("settings")
	if settings == nil || settings.Sub("gomodguard_v2") == nil {
		t.Fatal("presets/golangci-lint/.golangci.yml has no linters.settings.gomodguard_v2")
	}
	var out []blockedRow
	for _, m := range ordered.Maps(settings.Sub("gomodguard_v2").List("blocked")) {
		out = append(out, blockedRow{Module: m.Str("module"), Reason: m.Str("reason"), Recommends: ordered.Strings(m.List("recommendations")), Line: m.Line})
	}
	return out
}

// holdPreset is what is wrong between the register and the preset's blocked
// list, each problem named by the register's row, or by the entry's place in
// the list for an entry no row owns, and never by a module path.
func holdPreset(reg *layers.Register, blocked []blockedRow) []string {
	var bad []string
	want := reg.Blocked()
	for _, w := range want {
		i := slices.IndexFunc(blocked, func(b blockedRow) bool { return b.Module == w.Module })
		switch {
		case i < 0:
			bad = append(bad, "row "+w.Row+" is not carried: one module it replaces and bans has no blocked row in the preset")
		case blocked[i].Reason != w.Why || !slices.Equal(blocked[i].Recommends, []string{w.Instead}):
			bad = append(bad, "row "+w.Row+" is carried with a reason or a recommendation that is not the register's")
		}
	}
	for i, b := range blocked {
		if !slices.ContainsFunc(want, func(w layers.Blocked) bool { return w.Module == b.Module }) {
			bad = append(bad, "blocked entry "+strconv.Itoa(i+1)+" (line "+strconv.Itoa(b.Line)+") bans a module no chosen row of the register replaces and bans")
		}
		if b.Reason == "" || len(b.Recommends) == 0 {
			bad = append(bad, "blocked entry "+strconv.Itoa(i+1)+" (line "+strconv.Itoa(b.Line)+") has no reason or no recommendations")
		}
	}
	for _, l := range reg.Rows {
		if l.Banned() && slices.Contains(l.Linters, layers.LinterGomodguard) && !slices.ContainsFunc(want, func(w layers.Blocked) bool { return w.Row == l.Key }) {
			bad = append(bad, "row "+l.Key+" names gomodguard_v2 and replaces no module it denies")
		}
	}
	return bad
}

// TestThePresetCarriesTheModuleRowsOfTheRegisterAndNothingMore: both ways. A
// chosen banned row names its linter, and the gomodguard_v2 rows of the
// preset are the register's.
func TestThePresetCarriesTheModuleRowsOfTheRegisterAndNothingMore(t *testing.T) {
	t.Parallel()
	reg := theRegister(t)
	doc := lintDoc(t, "presets/golangci-lint/.golangci.yml")
	for _, l := range reg.Rows {
		if l.Banned() && len(l.Linters) == 0 {
			t.Errorf("row %s bans an import or a call and names no linter", l.Key)
		}
	}
	if len(reg.Blocked()) == 0 {
		t.Fatal("the register holds no module a gomodguard_v2 row replaces and bans, so nothing is held")
	}
	if bad := holdPreset(reg, presetBlocked(t, doc)); len(bad) > 0 {
		t.Errorf("the preset and the register disagree:\n%s", strings.Join(bad, "\n"))
	}
	enabled := ordered.Strings(doc.Sub("linters").List("enable"))
	for _, l := range []string{layers.LinterGomodguard, layers.LinterGomoddirective} {
		if !slices.Contains(enabled, l) {
			t.Errorf("the preset does not enable %s", l)
		}
	}
}

// TestARowTakenOutOfThePresetOrAddedToItTurnsTheRegisterTestRed is the
// plant of the test above: the test's own check, run on a preset with one
// blocked row removed, one changed and one added.
func TestARowTakenOutOfThePresetOrAddedToItTurnsTheRegisterTestRed(t *testing.T) {
	t.Parallel()
	reg := theRegister(t)
	blocked := presetBlocked(t, lintDoc(t, "presets/golangci-lint/.golangci.yml"))
	want := reg.Blocked()
	if len(blocked) != len(want) || len(want) < 3 {
		t.Fatalf("the preset blocks %d modules and the register %d: the plants below need them to agree", len(blocked), len(want))
	}
	dropped := slices.Delete(slices.Clone(blocked), 0, 1)
	if bad := holdPreset(reg, dropped); len(bad) != 1 || !strings.HasPrefix(bad[0], "row "+want[0].Row+" is not carried") {
		t.Errorf("a preset with the first blocked row removed gave %v", bad)
	}
	changed := slices.Clone(blocked)
	changed[1].Reason = "another reason"
	if bad := holdPreset(reg, changed); len(bad) != 1 || !strings.Contains(bad[0], "row "+want[1].Row+" is carried with a reason") {
		t.Errorf("a preset with a changed reason gave %v", bad)
	}
	extra := append(slices.Clone(blocked), blockedRow{Module: "example.test/not-in-the-register", Reason: "r", Recommends: []string{"x"}, Line: 9})
	if bad := holdPreset(reg, extra); len(bad) != 1 || !strings.Contains(bad[0], "blocked entry "+strconv.Itoa(len(extra))+" (line 9) bans a module no chosen row") || strings.Contains(bad[0], "not-in-the-register") {
		t.Errorf("a preset with an added row gave %v", bad)
	}
	if bad := holdPreset(reg, blocked); len(bad) != 0 {
		t.Errorf("the control, the preset as it is, gave %v", bad)
	}
}

// presetModule is the preset as a project's own .golangci.yml, over a module
// at the go and toolchain lines given, with files laid beside it. The imports
// are never resolved: gomodguard_v2 reads them as written.
func presetModule(t *testing.T, goMod string, files map[string]string) string {
	t.Helper()
	preset, err := os.ReadFile(filepath.Join(root, "presets", "golangci-lint", ".golangci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	all := map[string]string{".golangci.yml": string(preset), "go.mod": goMod}
	maps.Copy(all, files)
	return writeModule(t, all)
}

// lintMod runs the pinned golangci-lint with the two linters this file holds,
// the way make lint does, offline.
func lintMod(t *testing.T, bin, dir string) (string, bool) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "golangci-lint", "run", "--enable-only=gomodguard_v2,gomoddirectives",
		"--max-issues-per-linter=0", "--max-same-issues=0", "--uniq-by-line=false", "--allow-parallel-runners", "./...")
	// A planted module is never resolved: the cache is a cold folder of its own,
	// so a module the host's cache holds loads from nowhere, and the checksum
	// lookup is off, so go never dials sum.golang.org (#1093).
	cmd.Env = []string{"GOFLAGS=-mod=mod", "GOPROXY=off", "GOSUMDB=off", "GOMODCACHE=" + t.TempDir()}
	return lint(t, bin, dir, cmd)
}

const cleanGoMod = "module example.test/p\n\ngo 1.27.1\n\ntoolchain go1.27.4\n"

// TestAPlantedLocalReplaceFailsThePresetAndTheCleanControlPasses.
func TestAPlantedLocalReplaceFailsThePresetAndTheCleanControlPasses(t *testing.T) {
	bin := pinnedLint(t)
	files := map[string]string{"p.go": "// Package p is a plant.\npackage p\n"}
	if out, ok := lintMod(t, bin, presetModule(t, cleanGoMod, files)); !ok {
		t.Fatalf("the clean control fails the preset:\n%s", out)
	}
	out, ok := lintMod(t, bin, presetModule(t, cleanGoMod+"\nreplace example.test/x => ../x\n", files))
	if ok || !strings.Contains(out, "local replacement are not allowed") || !strings.Contains(out, "(gomoddirectives)") {
		t.Errorf("a planted replace => ../x gave ok=%v:\n%s", ok, out)
	}
}

// TestAPlantedGoOrToolchainLineThatBreaksThePatternFails: the go line and the
// toolchain line name a release to the patch.
func TestAPlantedGoOrToolchainLineThatBreaksThePatternFails(t *testing.T) {
	bin := pinnedLint(t)
	files := map[string]string{"p.go": "// Package p is a plant.\npackage p\n"}
	for name, goMod := range map[string]string{
		"a go line with no patch":        "module example.test/p\n\ngo 1.27\n\ntoolchain go1.27.4\n",
		"a toolchain line with no patch": "module example.test/p\n\ngo 1.27.1\n\ntoolchain go1.27\n",
	} {
		out, ok := lintMod(t, bin, presetModule(t, goMod, files))
		if ok || !strings.Contains(out, "(gomoddirectives)") {
			t.Errorf("%s gave ok=%v:\n%s", name, ok, out)
		}
	}
}

// TestEachBlockedModuleOfThePresetFailsWithItsRecommendation: one plant for
// each blocked row, a file that imports the module, and the report names the
// module, the row's recommendation and its reason.
func TestEachBlockedModuleOfThePresetFailsWithItsRecommendation(t *testing.T) {
	bin := pinnedLint(t)
	reg := theRegister(t)
	blocked := reg.Blocked()
	files := map[string]string{}
	for i, b := range blocked {
		files["b"+strconv.Itoa(i)+"/x.go"] = "// Package b" + strconv.Itoa(i) + " is the plant of row " + b.Row + ".\npackage b" + strconv.Itoa(i) + "\n\nimport _ \"" + b.Module + "\"\n"
	}
	goMod := cleanGoMod + "\nrequire (\n"
	for _, b := range blocked {
		goMod += "\t" + b.Module + " v1.0.0\n"
	}
	out, ok := lintMod(t, bin, presetModule(t, goMod+")\n", files))
	if ok {
		t.Fatalf("the plants pass the preset:\n%s", out)
	}
	for i, b := range blocked {
		line := "b" + strconv.Itoa(i) + "/x.go:4:"
		want := "import of package `" + b.Module + "` is blocked because the module is in the blocked modules list. `" + b.Instead + "` is a recommended module. " + b.Why + "."
		if !slices.ContainsFunc(strings.Split(out, "\n"), func(l string) bool { return strings.HasPrefix(l, line) && strings.Contains(l, want) }) {
			t.Errorf("the plant of row %s is not reported with its recommendation and reason", b.Row)
		}
	}
	if t.Failed() {
		t.Logf("the lint said:\n%s", out)
	}
	if clean, ok := lintMod(t, bin, presetModule(t, cleanGoMod, map[string]string{"p.go": "// Package p is the control.\npackage p\n"})); !ok {
		t.Errorf("the clean control fails the preset:\n%s", clean)
	}
}

// libRulePrefix begins the name of each depguard rule of the preset that a row
// of the register owns: lib-<row>.
const libRulePrefix = "lib-"

// libReasonPrefix begins the reason of each depguard or forbidigo row of the
// preset that a row of the register owns. It opens with the rule's id and a
// colon, so a project's exclusion of one row's reports is confined to that
// rule and voids no shared ban (arch.shared.8).
const libReasonPrefix = "supply.intake.6: row "

// libReason is the reason the preset gives for a row's import or call: the
// rule, the row, the register's why and the row's own library.
func libReason(row, why, instead string) string {
	return libReasonPrefix + row + ": " + why + "; instead: " + instead
}

// importRow is one deny entry of a depguard rule of the preset.
type importRow struct {
	Pkg, Desc string
	Line      int
}

// callRow is one entry of the preset's forbidigo list.
type callRow struct {
	Row, Pattern, Pkg, Msg string
	Line                   int
}

// presetImports is the preset's depguard rules a row of the register owns, by
// name, and the line each rule's name is on.
func presetImports(t *testing.T, doc *ordered.Map) (map[string][]importRow, map[string]int) {
	t.Helper()
	rules := doc.Sub("linters").Sub("settings").Sub("depguard").Sub("rules")
	if rules == nil {
		t.Fatal("presets/golangci-lint/.golangci.yml has no linters.settings.depguard.rules")
	}
	got, lines := map[string][]importRow{}, map[string]int{}
	for _, name := range rules.Keys() {
		if !strings.HasPrefix(name, libRulePrefix) {
			continue
		}
		lines[name] = rules.KeyLine(name)
		got[name] = []importRow{}
		r := rules.Sub(name)
		if r == nil {
			continue
		}
		for _, m := range ordered.Maps(r.List("deny")) {
			got[name] = append(got[name], importRow{Pkg: m.Str("pkg"), Desc: m.Str("desc"), Line: m.Line})
		}
	}
	return got, lines
}

// presetCalls is the preset's forbidigo entries a row of the register owns:
// those whose message begins with the register's rule.
func presetCalls(t *testing.T, doc *ordered.Map) []callRow {
	t.Helper()
	f := doc.Sub("linters").Sub("settings").Sub("forbidigo")
	if f == nil {
		t.Fatal("presets/golangci-lint/.golangci.yml has no linters.settings.forbidigo")
	}
	var out []callRow
	for _, m := range ordered.Maps(f.List("forbid")) {
		if strings.HasPrefix(m.Str("msg"), "supply.intake.6") {
			out = append(out, callRow{Pattern: m.Str("pattern"), Pkg: m.Str("pkg"), Msg: m.Str("msg"), Line: m.Line})
		}
	}
	return out
}

// wantImports is the depguard rules the register's chosen banned rows that
// name depguard give the preset, by name, and the rows whose denied import has
// no why: the why of the module it replaces, or the row's ban.note where it
// replaces none.
func wantImports(reg *layers.Register) (map[string][]importRow, []string) {
	want := map[string][]importRow{}
	var bad []string
	for _, l := range reg.Rows {
		if !l.Banned() || !slices.Contains(l.Linters, layers.LinterDepguard) {
			continue
		}
		name := libRulePrefix + l.Key
		want[name] = []importRow{}
		for _, d := range l.Deny {
			why := l.BanNote
			if i := slices.IndexFunc(l.Replaces, func(r layers.Replaced) bool { return r.Name == d }); i >= 0 {
				why = l.Replaces[i].Why
			}
			if why == "" {
				bad = append(bad, "row "+l.Key+" denies an import with no why: it replaces no such module and has no ban.note")
			}
			want[name] = append(want[name], importRow{Pkg: d, Desc: libReason(l.Key, why, l.Module)})
		}
	}
	return want, bad
}

// wantCalls is the forbidigo entries the register's chosen banned rows that
// name forbidigo give the preset: each pattern, its package as an anchored
// regexp, and the row's job as the reason.
func wantCalls(reg *layers.Register) []callRow {
	var out []callRow
	for _, l := range reg.Rows {
		if !l.Banned() || !slices.Contains(l.Linters, layers.LinterForbidigo) {
			continue
		}
		for _, f := range l.Forbid {
			pkg := ""
			if f.Pkg != "" {
				pkg = "^" + regexp.QuoteMeta(f.Pkg) + "$"
			}
			out = append(out, callRow{Row: l.Key, Pattern: f.Pattern, Pkg: pkg, Msg: libReason(l.Key, l.Job, l.Module)})
		}
	}
	return out
}

// holdImports is what is wrong between the register's depguard rules and the
// preset's, each problem named by the register's row or by the preset's rule
// and line, and never by an import path.
func holdImports(want, got map[string][]importRow, lines map[string]int) []string {
	var bad []string
	for _, name := range slices.Sorted(maps.Keys(want)) {
		row := strings.TrimPrefix(name, libRulePrefix)
		g, ok := got[name]
		if !ok {
			bad = append(bad, "row "+row+" is not carried: the preset has no depguard rule "+name)
			continue
		}
		for _, w := range want[name] {
			i := slices.IndexFunc(g, func(e importRow) bool { return e.Pkg == w.Pkg })
			switch {
			case i < 0:
				bad = append(bad, "row "+row+" is not carried: one import it denies has no deny entry in rule "+name)
			case g[i].Desc != w.Desc:
				bad = append(bad, "row "+row+" is carried with a reason or a replacement that is not the register's")
			}
		}
		for i, e := range g {
			if !slices.ContainsFunc(want[name], func(w importRow) bool { return w.Pkg == e.Pkg }) {
				bad = append(bad, "rule "+name+" entry "+strconv.Itoa(i+1)+" (line "+strconv.Itoa(e.Line)+") denies an import row "+row+" does not")
			}
		}
	}
	for _, name := range slices.Sorted(maps.Keys(got)) {
		if _, ok := want[name]; !ok {
			bad = append(bad, "depguard rule "+name+" (line "+strconv.Itoa(lines[name])+") is no chosen banned row of the register that names depguard")
		}
	}
	return bad
}

// holdCalls is what is wrong between the register's forbidden calls and the
// preset's forbidigo entries that cite the register.
func holdCalls(want, got []callRow) []string {
	var bad []string
	same := func(a, b callRow) bool { return a.Pattern == b.Pattern && a.Pkg == b.Pkg }
	for _, w := range want {
		i := slices.IndexFunc(got, func(g callRow) bool { return same(g, w) })
		switch {
		case i < 0:
			bad = append(bad, "row "+w.Row+" is not carried: one call it forbids has no forbidigo entry")
		case got[i].Msg != w.Msg:
			bad = append(bad, "row "+w.Row+" is carried with a reason or a replacement that is not the register's")
		}
	}
	for _, g := range got {
		if !slices.ContainsFunc(want, func(w callRow) bool { return same(g, w) }) {
			bad = append(bad, "forbidigo entry (line "+strconv.Itoa(g.Line)+") cites supply.intake.6 and forbids a call no chosen row of the register forbids")
		}
	}
	return bad
}

// TestThePresetCarriesTheImportAndCallRowsOfTheRegisterAndNothingMore: both
// ways, for depguard and forbidigo, as for gomodguard_v2 above.
func TestThePresetCarriesTheImportAndCallRowsOfTheRegisterAndNothingMore(t *testing.T) {
	t.Parallel()
	reg := theRegister(t)
	doc := lintDoc(t, "presets/golangci-lint/.golangci.yml")
	want, bad := wantImports(reg)
	if len(want) == 0 {
		t.Fatal("the register holds no chosen row whose ban names depguard, so nothing is held")
	}
	calls := wantCalls(reg)
	if len(calls) == 0 {
		t.Fatal("the register holds no chosen row whose ban names forbidigo, so nothing is held")
	}
	got, lines := presetImports(t, doc)
	bad = append(bad, holdImports(want, got, lines)...)
	bad = append(bad, holdCalls(calls, presetCalls(t, doc))...)
	if len(bad) > 0 {
		t.Errorf("the preset and the register disagree:\n%s", strings.Join(bad, "\n"))
	}
	enabled := ordered.Strings(doc.Sub("linters").List("enable"))
	for _, l := range []string{layers.LinterDepguard, layers.LinterForbidigo} {
		if !slices.Contains(enabled, l) {
			t.Errorf("the preset does not enable %s", l)
		}
	}
}

// cloneImports is a deep copy of a preset's depguard rules.
func cloneImports(m map[string][]importRow) map[string][]importRow {
	out := map[string][]importRow{}
	for k, v := range m {
		out[k] = slices.Clone(v)
	}
	return out
}

// TestADepguardOrForbidigoRowTakenOutOrAddedTurnsTheRegisterTestRed is the
// plant of the test above: its own checks, run on a preset with a rule, an
// entry or a call taken out, a reason changed, and one added, and on a
// register row whose denied import has no why.
func TestADepguardOrForbidigoRowTakenOutOrAddedTurnsTheRegisterTestRed(t *testing.T) {
	t.Parallel()
	reg := theRegister(t)
	doc := lintDoc(t, "presets/golangci-lint/.golangci.yml")
	want, _ := wantImports(reg)
	got, lines := presetImports(t, doc)
	names := slices.Sorted(maps.Keys(want))
	multi := slices.IndexFunc(names, func(n string) bool { return len(want[n]) > 1 })
	if len(names) < 2 || multi < 0 {
		t.Fatalf("the register gives %d depguard rules, none with two imports: the plants below need more", len(names))
	}
	first, two := names[0], names[multi]
	row := func(n string) string { return strings.TrimPrefix(n, libRulePrefix) }
	one := func(what string, bad []string, want string) {
		t.Helper()
		if len(bad) != 1 || !strings.Contains(bad[0], want) {
			t.Errorf("%s gave %v", what, bad)
		}
	}
	dropped := cloneImports(got)
	delete(dropped, first)
	one("a preset with a rule removed", holdImports(want, dropped, lines), "row "+row(first)+" is not carried: the preset has no depguard rule")
	entry := cloneImports(got)
	entry[two] = slices.Delete(entry[two], 0, 1)
	one("a preset with a denied import removed", holdImports(want, entry, lines), "row "+row(two)+" is not carried: one import it denies")
	changed := cloneImports(got)
	changed[two][1].Desc = "another reason"
	one("a preset with a changed reason", holdImports(want, changed, lines), "row "+row(two)+" is carried with a reason")
	added := cloneImports(got)
	added[first] = append(added[first], importRow{Pkg: "example.test/not-in-the-register", Desc: "r", Line: 9})
	bad := holdImports(want, added, lines)
	one("a preset with an import added to a rule", bad, "(line 9) denies an import row "+row(first)+" does not")
	if strings.Contains(strings.Join(bad, ""), "not-in-the-register") {
		t.Errorf("a problem names the import path: %v", bad)
	}
	rule := cloneImports(got)
	rule["lib-not-a-row"] = []importRow{{Pkg: "example.test/x", Desc: "r", Line: 9}}
	one("a preset with a rule added", holdImports(want, rule, lines), "depguard rule lib-not-a-row (line 0) is no chosen banned row")
	if bad := holdImports(want, got, lines); len(bad) != 0 {
		t.Errorf("the control, the preset's depguard rules as they are, gave %v", bad)
	}

	calls, gotCalls := wantCalls(reg), presetCalls(t, doc)
	if len(calls) == 0 || len(gotCalls) != len(calls) {
		t.Fatalf("the preset has %d forbidigo entries of the register and the register %d: the plants below need them to agree", len(gotCalls), len(calls))
	}
	one("a preset with a forbidden call removed", holdCalls(calls, slices.Delete(slices.Clone(gotCalls), 0, 1)), "row "+calls[0].Row+" is not carried: one call it forbids")
	msg := slices.Clone(gotCalls)
	msg[0].Msg = "another reason"
	one("a preset with a changed call reason", holdCalls(calls, msg), "row "+calls[0].Row+" is carried with a reason")
	one("a preset with a call added", holdCalls(calls, append(slices.Clone(gotCalls), callRow{Pattern: `^x\.Y$`, Msg: "supply.intake.6: row none: r", Line: 9})), "forbidigo entry (line 9) cites supply.intake.6")
	if bad := holdCalls(calls, gotCalls); len(bad) != 0 {
		t.Errorf("the control, the preset's forbidigo entries as they are, gave %v", bad)
	}

	noted := slices.IndexFunc(reg.Rows, func(l layers.Library) bool {
		return l.Banned() && slices.Contains(l.Linters, layers.LinterDepguard) && l.BanNote != "" &&
			slices.ContainsFunc(l.Deny, func(d string) bool {
				return !slices.ContainsFunc(l.Replaces, func(r layers.Replaced) bool { return r.Name == d })
			})
	})
	if noted < 0 {
		t.Fatal("no chosen depguard row denies an import it does not replace, so the why's plant has no row")
	}
	bare := &layers.Register{Rows: slices.Clone(reg.Rows)}
	bare.Rows[noted].BanNote = ""
	if _, bad := wantImports(bare); len(bad) == 0 || !strings.HasPrefix(bad[0], "row "+bare.Rows[noted].Key+" denies an import with no why") {
		t.Errorf("a row with its ban.note taken out gave %v", bad)
	}
}

// stubVersion is the version a planted go.mod requires a stand-in module at:
// the major version its path names, or v0.
func stubVersion(path string) string {
	last := path[strings.LastIndex(path, "/")+1:]
	if i := strings.LastIndex(last, ".v"); strings.HasPrefix(path, "gopkg.in/") && i >= 0 {
		return last[i+1:] + ".0.0"
	}
	if regexp.MustCompile(`^v[2-9][0-9]*$`).MatchString(last) {
		return last + ".0.0"
	}
	return "v0.0.0"
}

// callPlant is a call a forbidigo pattern refuses: the package it imports and
// the call, and, for a package outside the standard library, the module that
// holds it and a stand-in of the package's source with the called function.
type callPlant struct {
	imp, call, module, stub string
}

// callPlants is a call that each forbidigo pattern of the register refuses;
// a pattern added to the register needs its plant here.
func callPlants() map[string]callPlant {
	return map[string]callPlant{
		`^syscall\.Umask$`: {imp: "syscall", call: "syscall.Umask(0)"},
		`^unix\.Umask$`: {imp: "golang.org/x/sys/unix", call: "unix.Umask(0)", module: "golang.org/x/sys",
			stub: "// Package unix is a stand-in.\npackage unix\n\n// Umask is a stand-in.\nfunc Umask(mask int) int { return mask }\n"},
		`^syscall\.Flock$`: {imp: "syscall", call: "syscall.Flock(0, 0)"},
		`^unix\.Flock$`: {imp: "golang.org/x/sys/unix", call: "unix.Flock(0, 0)", module: "golang.org/x/sys",
			stub: "package unix\n\n// Flock is a stand-in.\nfunc Flock(fd, how int) error { return nil }\n"},
	}
}

// addCallPlants lays one plant for each call, c<i>/x.go with the call on its
// line 7, and a stand-in module for each module a plant's package is outside
// the standard library in, each plant's stand-in source a file of its own;
// it gives the require and replace lines the stand-ins need.
func addCallPlants(t *testing.T, files map[string]string, calls []callRow) (require, replace string) {
	t.Helper()
	var modules []string
	for i, c := range calls {
		plant, ok := callPlants()[c.Pattern]
		if !ok {
			t.Fatalf("the call row %s forbids has no plant in callPlants", c.Row)
		}
		files["c"+strconv.Itoa(i)+"/x.go"] = "// Package c" + strconv.Itoa(i) + " is a plant of a forbidden call.\npackage c" + strconv.Itoa(i) + "\n\nimport \"" + plant.imp + "\"\n\n// Use makes the call.\nfunc Use() { _ = " + plant.call + " }\n"
		if plant.module == "" {
			continue
		}
		m := slices.Index(modules, plant.module)
		if m < 0 {
			m = len(modules)
			modules = append(modules, plant.module)
			files["stub/m"+strconv.Itoa(m)+"/go.mod"] = "module " + plant.module + "\n\ngo 1.23.0\n"
			require += "\t" + plant.module + " " + stubVersion(plant.module) + "\n"
			replace += "\t" + plant.module + " => ./stub/m" + strconv.Itoa(m) + "\n"
		}
		files["stub/m"+strconv.Itoa(m)+"/"+strings.TrimPrefix(plant.imp, plant.module+"/")+"/f"+strconv.Itoa(i)+".go"] = plant.stub
	}
	return require, replace
}

// TestEachDeniedImportAndForbiddenCallOfThePresetFailsWithItsReason: one
// plant for each import the register's depguard rows deny and each call its
// forbidigo rows forbid, through the preset with the pinned golangci-lint; each
// is reported with its rule's reason, and the control, a package that imports
// and calls nothing, is not reported. The imports are stand-in modules a
// replace points at, so the plants build with no network; the run is for
// linux, where the forbidden call is declared.
func TestEachDeniedImportAndForbiddenCallOfThePresetFailsWithItsReason(t *testing.T) {
	bin := pinnedLint(t)
	reg := theRegister(t)
	want, _ := wantImports(reg)
	calls := wantCalls(reg)
	preset, err := os.ReadFile(filepath.Join(root, "presets", "golangci-lint", ".golangci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{".golangci.yml": string(preset)}
	var paths []string
	for _, rows := range want {
		for _, r := range rows {
			if !slices.Contains(paths, r.Pkg) {
				paths = append(paths, r.Pkg)
			}
		}
	}
	slices.Sort(paths)
	goMod := "module example.test/p\n\ngo 1.23.0\n\nrequire (\n"
	replace := "replace (\n"
	for i, p := range paths {
		stub := "stub/s" + strconv.Itoa(i)
		files[stub+"/go.mod"] = "module " + p + "\n\ngo 1.23.0\n"
		files[stub+"/s.go"] = "// Package stub is a stand-in.\npackage stub\n"
		files["d"+strconv.Itoa(i)+"/x.go"] = "// Package d" + strconv.Itoa(i) + " is a plant of a denied import.\npackage d" + strconv.Itoa(i) + "\n\nimport _ \"" + p + "\"\n"
		goMod += "\t" + p + " " + stubVersion(p) + "\n"
		replace += "\t" + p + " => ./" + stub + "\n"
	}
	req, rep := addCallPlants(t, files, calls)
	files["go.mod"] = goMod + req + ")\n\n" + replace + rep + ")\n"
	files["plain/x.go"] = "// Package plain is the control: it imports and calls nothing.\npackage plain\n\n// Is is a plain value.\nconst Is = 1\n"
	cmd := exec.CommandContext(t.Context(), "golangci-lint", "run", "--enable-only=depguard,forbidigo",
		"--max-issues-per-linter=0", "--max-same-issues=0", "--uniq-by-line=false", "--allow-parallel-runners", "./...")
	cmd.Env = []string{"GOFLAGS=-mod=mod", "GOPROXY=off", "GOOS=linux"}
	out, ok := lint(t, bin, writeModule(t, files), cmd)
	if ok {
		t.Fatalf("the plants pass the preset:\n%s", out)
	}
	lines := strings.Split(out, "\n")
	reported := func(prefix string, says ...string) bool {
		return slices.ContainsFunc(lines, func(l string) bool {
			if !strings.HasPrefix(l, prefix) {
				return false
			}
			for _, s := range says {
				if !strings.Contains(l, s) {
					return false
				}
			}
			return true
		})
	}
	for _, name := range slices.Sorted(maps.Keys(want)) {
		for _, r := range want[name] {
			line := "d" + strconv.Itoa(slices.Index(paths, r.Pkg)) + "/x.go:4:"
			if !reported(line, "'"+name+"'", r.Desc, "(depguard)") {
				t.Errorf("an import row %s denies is not reported by rule %s with its reason", strings.TrimPrefix(name, libRulePrefix), name)
			}
		}
	}
	for i, c := range calls {
		if !reported("c"+strconv.Itoa(i)+"/x.go:7:", c.Msg, "(forbidigo)") {
			t.Errorf("the call row %s forbids is not reported with its reason", c.Row)
		}
	}
	if reported("plain/") {
		t.Errorf("the control is reported")
	}
	if t.Failed() {
		t.Logf("the lint said:\n%s", out)
	}
}

// TestAYamlV2ImportIsToldTheYamlRowAndACharmImportIsNotRefused: a yaml.v2
// import is reported once, by lib-yaml with the register's reason and its own
// instead, and no rule tells it to use the crawler's xpath library (the
// crawl-pins row denies nothing); an import of the Charm command frame is not
// reported by any rule, as a product may import Charm directly
// (gragas#1603).
func TestAYamlV2ImportIsToldTheYamlRowAndACharmImportIsNotRefused(t *testing.T) {
	bin := pinnedLint(t)
	reg := theRegister(t)
	want, _ := wantImports(reg)
	reasonOf := func(rule, pkg string) string {
		for _, r := range want[rule] {
			if r.Pkg == pkg {
				return r.Desc
			}
		}
		t.Fatalf("rule %s has no import of %s in the register", rule, pkg)
		return ""
	}
	preset, err := os.ReadFile(filepath.Join(root, "presets", "golangci-lint", ".golangci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := want["lib-cli-frame"]; ok {
		t.Error("the register still has a lib-cli-frame row of imports")
	}
	const frame = "charm.land/fang/v2"
	files := map[string]string{
		".golangci.yml": string(preset),
		"go.mod": "module example.test/p\n\ngo 1.23.0\n\nrequire (\n\tgopkg.in/yaml.v2 v2.0.0\n\t" + frame + " v2.0.0\n)\n\n" +
			"replace (\n\tgopkg.in/yaml.v2 => ./stub/yaml\n\t" + frame + " => ./stub/fang\n)\n",
		"stub/yaml/go.mod": "module gopkg.in/yaml.v2\n\ngo 1.23.0\n",
		"stub/yaml/s.go":   "// Package stub is a stand-in.\npackage stub\n",
		"stub/fang/go.mod": "module " + frame + "\n\ngo 1.23.0\n",
		"stub/fang/s.go":   "// Package stub is a stand-in.\npackage stub\n",
		"yaml2/x.go":       "// Package yaml2 plants a yaml.v2 import.\npackage yaml2\n\nimport _ \"gopkg.in/yaml.v2\"\n",
		"other/x.go":       "// Package other imports the command frame.\npackage other\n\nimport _ \"" + frame + "\"\n",
	}
	cmd := exec.CommandContext(t.Context(), "golangci-lint", "run", "--enable-only=depguard",
		"--max-issues-per-linter=0", "--max-same-issues=0", "--uniq-by-line=false", "--allow-parallel-runners", "./...")
	cmd.Env = []string{"GOFLAGS=-mod=mod", "GOPROXY=off"}
	out, ok := lint(t, bin, writeModule(t, files), cmd)
	if ok {
		t.Fatalf("the plants pass the preset:\n%s", out)
	}
	var yamlLines, otherLines []string
	for l := range strings.SplitSeq(out, "\n") {
		switch {
		case strings.HasPrefix(l, "yaml2/"):
			yamlLines = append(yamlLines, l)
		case strings.HasPrefix(l, "other/"):
			otherLines = append(otherLines, l)
		}
	}
	if len(yamlLines) != 1 || !strings.Contains(yamlLines[0], reasonOf("lib-yaml", "gopkg.in/yaml.v2")) {
		t.Errorf("the yaml.v2 import is not reported once, by lib-yaml with its reason: %q", yamlLines)
	}
	if strings.Contains(strings.Join(yamlLines, "\n"), "antchfx/xpath") {
		t.Errorf("the yaml.v2 import is told to use the xpath library: %q", yamlLines)
	}
	if len(otherLines) != 0 {
		t.Errorf("an import of the Charm command frame is reported: %q", otherLines)
	}
	if t.Failed() {
		t.Logf("the lint said:\n%s", out)
	}
}

// TestAnExecCmdLiteralRunInANonTestFileIsRefusedAndLookPathIsNot: a program
// started with a literal &exec.Cmd{Path: ...} and .Run(), with no call of
// exec.Command, is reported by cli.process.1 in a file that is not a test; the
// same in a test file is spared, and exec.LookPath, which starts nothing, is
// reported nowhere (gragas#1603).
func TestAnExecCmdLiteralRunInANonTestFileIsRefusedAndLookPathIsNot(t *testing.T) {
	bin := pinnedLint(t)
	preset, err := os.ReadFile(filepath.Join(root, "presets", "golangci-lint", ".golangci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	const body = "import \"os/exec\"\n\n// Start runs a program with a literal.\nfunc Start() error {\n\tp, err := exec.LookPath(\"sh\")\n\tif err != nil {\n\t\treturn err\n\t}\n\tc := &exec.Cmd{Path: p, Args: []string{p}}\n\treturn c.Run()\n}\n"
	files := map[string]string{
		".golangci.yml": string(preset),
		"go.mod":        "module example.test/p\n\ngo 1.23.0\n",
		"lit/x.go":      "// Package lit starts a program with an exec.Cmd literal.\npackage lit\n\n" + body,
		"lit/x_test.go": "package lit\n\n" + strings.Replace(body, "Start", "StartInTest", 2),
		"look/x.go":     "// Package look only looks a name up.\npackage look\n\nimport \"os/exec\"\n\n// Where is the path of sh.\nfunc Where() (string, error) { return exec.LookPath(\"sh\") }\n",
	}
	cmd := exec.CommandContext(t.Context(), "golangci-lint", "run", "--enable-only=forbidigo",
		"--max-issues-per-linter=0", "--max-same-issues=0", "--uniq-by-line=false", "--allow-parallel-runners", "./...")
	cmd.Env = []string{"GOFLAGS=-mod=mod", "GOPROXY=off"}
	out, _ := lint(t, bin, writeModule(t, files), cmd)
	var lit, test, look int
	for l := range strings.SplitSeq(out, "\n") {
		switch {
		case strings.HasPrefix(l, "lit/x.go:") && strings.Contains(l, "cli.process.1"):
			lit++
		case strings.HasPrefix(l, "lit/x_test.go:"):
			test++
		case strings.HasPrefix(l, "look/"):
			look++
		}
	}
	if lit == 0 {
		t.Errorf("the exec.Cmd literal in a non-test file is not reported by cli.process.1")
	}
	if test != 0 || look != 0 {
		t.Errorf("a test file's literal (%d) or exec.LookPath (%d) is reported", test, look)
	}
	if t.Failed() {
		t.Logf("the lint said:\n%s", out)
	}
}

// TestSyscallSigtermIsRefusedEverywhere: syscall.SIGTERM in any file is
// reported by go.concurrency.4, a test file included, by the preset and by
// gragas's own config, the signal journey's file (internal/journeys/
// signals_test.go) too: since #1546 it names no syscall, and no path is spared
// (gragas#1603).
func TestSyscallSigtermIsRefusedEverywhere(t *testing.T) {
	bin := pinnedLint(t)
	const body = "import \"syscall\"\n\n// Stop is the signal.\nvar Stop = syscall.SIGTERM\n"
	for name, cfg := range map[string]string{"the preset": filepath.Join("presets", "golangci-lint", ".golangci.yml"), "gragas's config": ".golangci.yml"} {
		conf, err := os.ReadFile(filepath.Join(root, cfg))
		if err != nil {
			t.Fatal(err)
		}
		files := map[string]string{
			".golangci.yml":                     string(conf),
			"go.mod":                            "module example.test/p\n\ngo 1.23.0\n",
			"other/x.go":                        "// Package other names the signal.\npackage other\n\n" + body,
			"other/x_test.go":                   "package other\n\n" + strings.Replace(body, "Stop", "StopInTest", 2),
			"internal/journeys/signals_test.go": "package journeys\n\n" + strings.Replace(body, "Stop", "StopInJourney", 2),
			"internal/journeys/another_test.go": "package journeys\n\n" + strings.Replace(body, "Stop", "StopAnother", 2),
		}
		cmd := exec.CommandContext(t.Context(), "golangci-lint", "run", "--enable-only=forbidigo",
			"--max-issues-per-linter=0", "--max-same-issues=0", "--uniq-by-line=false", "--allow-parallel-runners", "./...")
		cmd.Env = []string{"GOFLAGS=-mod=mod", "GOPROXY=off"}
		out, _ := lint(t, bin, writeModule(t, files), cmd)
		reported := map[string]bool{}
		for l := range strings.SplitSeq(out, "\n") {
			if strings.Contains(l, "go.concurrency.4") {
				file, _, _ := strings.Cut(l, ":")
				reported[file] = true
			}
		}
		for _, f := range []string{"other/x.go", "other/x_test.go", "internal/journeys/signals_test.go", "internal/journeys/another_test.go"} {
			if !reported[f] {
				t.Errorf("%s: %s names syscall.SIGTERM and is not reported", name, f)
			}
		}
		if t.Failed() {
			t.Logf("the lint said:\n%s", out)
		}
	}
}

// TestSyscallAndXSysAreRefusedInEveryFileButTheThreeNamed: a file that imports
// syscall or golang.org/x/sys is refused, a test file and a file of
// the host layer included, by the preset and by gragas's own config; gragas's
// config spares exactly the three files its overrides name, each until the OS
// audit row that replaces it lands, and not a fourth: the pnpm test's went with
// gragas#1605, so a syscall import there is refused again. The run is for windows,
// where a file of the host layer that names x/sys/windows is built.
func TestSyscallAndXSysAreRefusedInEveryFileButTheThreeNamed(t *testing.T) {
	bin := pinnedLint(t)
	spared := map[string]string{
		"internal/host/git/remote_deadline_unix_test.go": "syscall",
		"internal/host/trust/owned_unix_test.go":         "syscall",
		"internal/journeys/blocked_test.go":              "syscall",
	}
	refused := map[string]string{
		"internal/release_age_pnpm_test.go": "syscall",
		"other/x.go":                        "syscall",
		"other/x_test.go":                   "syscall",
		"internal/host/fs/x.go":             "syscall",
		"internal/host/fs/y.go":             "golang.org/x/sys",
		"internal/host/link/more.go":        "golang.org/x/sys/windows",
		"internal/journeys/more_test.go":    "syscall",
		"internal/more_test.go":             "syscall",
	}
	for name, cfg := range map[string]string{"the preset": filepath.Join("presets", "golangci-lint", ".golangci.yml"), "gragas's config": ".golangci.yml"} {
		conf, err := os.ReadFile(filepath.Join(root, cfg))
		if err != nil {
			t.Fatal(err)
		}
		files := map[string]string{
			".golangci.yml":     string(conf),
			"go.mod":            "module example.test/p\n\ngo 1.23.0\n\nrequire golang.org/x/sys v0.0.0\n\nreplace golang.org/x/sys => ./stub\n",
			"stub/go.mod":       "module golang.org/x/sys\n\ngo 1.23.0\n",
			"stub/s.go":         "// Package stub is a stand-in.\npackage stub\n",
			"stub/windows/w.go": "// Package windows is a stand-in.\npackage windows\n",
		}
		all := map[string]string{}
		maps.Copy(all, spared)
		maps.Copy(all, refused)
		for p, imp := range all {
			files[p] = "// Package " + path.Base(path.Dir(p)) + " is a plant of an import.\npackage " + path.Base(path.Dir(p)) + "\n\nimport _ \"" + imp + "\"\n"
		}
		cmd := exec.CommandContext(t.Context(), "golangci-lint", "run", "--enable-only=depguard",
			"--max-issues-per-linter=0", "--max-same-issues=0", "--uniq-by-line=false", "--allow-parallel-runners", "./...")
		cmd.Env = []string{"GOFLAGS=-mod=mod", "GOPROXY=off", "GOOS=windows"}
		out, _ := lint(t, bin, writeModule(t, files), cmd)
		reported := map[string]bool{}
		for l := range strings.SplitSeq(out, "\n") {
			if strings.Contains(l, "no-os-packages") {
				file, _, _ := strings.Cut(l, ":")
				reported[file] = true
			}
		}
		for p := range all {
			_, isSpared := spared[p]
			want := name == "the preset" || !isSpared
			if reported[p] != want {
				t.Errorf("%s: %s is reported = %v, want %v", name, p, reported[p], want)
			}
		}
		if t.Failed() {
			t.Logf("the lint said:\n%s", out)
		}
	}
}
