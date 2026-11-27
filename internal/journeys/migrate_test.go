package journeys

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/birdtable/go-console/uitest"
)

// migrateRun runs migrate over dir with the real git, which names who migrated.
func migrateRun(t *testing.T, dir, cols string, args ...string) result {
	t.Helper()
	env, _ := folders(t)
	git := journeyGit(t)
	settings, _ := settingsFile(t, "version: 1\nprograms:\n  git: "+strconv.Quote(git)+"\n") // a Windows path holds backslashes, which a quoted scalar escapes
	env["GRAGAS_CONFIG"] = settings
	env["COLUMNS"] = cols
	env["GRAGAS_NAMES_FILE"] = ""
	return run(t, dir, env, append([]string{"migrate"}, args...)...)
}

func migrateFlags() []string { return []string{"--name", "demo", "--owner", "team-a"} }

// checksum is one digest of every file below dir, by path and content.
func checksum(t *testing.T, dir string) string {
	t.Helper()
	h := sha256.New()
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		h.Write([]byte(rel + "\x00"))
		h.Write(b)
		return nil
	})
	if err != nil {
		t.Fatalf("checksum %s: %v", dir, err)
	}
	return hex.EncodeToString(h.Sum(nil))
}

var (
	carriedLine = regexp.MustCompile(`carried: ([\d,]+) rules?, ([\d,]+) bindings?, ([\d,]+) sources?, ([\d,]+) guides?, ([\d,]+) kinds?`)
	proseLine   = regexp.MustCompile(`([\d,]+) prose findings? for validate`)
	heldLine    = regexp.MustCompile(`([\d,]+)\s+migrated\s+rules?\s+held\s+from\s+the\s+prose\s+lint`)
	strengthsAt = regexp.MustCompile(`strengths: Hard to must, ([\d,]+) rules?; Answered, Session and Record to should, ([\d,]+) rules?; Lens and Review to may, ([\d,]+) rules?;`)
	relationsAt = regexp.MustCompile(`relations: see to relates, ([\d,]+) relations?; refines to relates, ([\d,]+) relations?; conflicts to conflicts, ([\d,]+) relations?; the parked file keeps each word that went to relates, and the ([\d,]+) winners? named`)
	parkedAt    = regexp.MustCompile(`parked: ([\d,]+) lines? in (\S+),`)
	notCarried  = regexp.MustCompile(`(?m)^\s*not carried: ([^:]+):`)
)

// count reads a number the screen prints with its thousands commas.
func count(s string) string { return strings.ReplaceAll(s, ",", "") }

// literalRelations counts the relations of a v2 folder by the how word written,
// see, refines and conflicts, and the winners named, read from the text, not
// through the program under test.
func literalRelations(t *testing.T, dir string) [4]int {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*", "*", "rules", "*.yaml"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no rule files below %s: %v", dir, err)
	}
	at := map[string]int{"how: see": 0, "how: refines": 1, "how: conflicts": 2}
	var n [4]int
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for line := range strings.SplitSeq(string(b), "\n") {
			w, ok := strings.CutPrefix(line, "        ")
			if i, how := at[w]; how && ok {
				n[i]++
			} else if ok && strings.HasPrefix(w, "wins: ") {
				n[3]++
			}
		}
	}
	return n
}

// literalStrengths counts the rules of a v2 folder by the strength format 3
// gives them, must, should and may, by reading each rule's strength word in
// the text and none of the migration.
func literalStrengths(t *testing.T, dir string) [3]int {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*", "*", "rules", "*.yaml"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no rule files below %s: %v", dir, err)
	}
	to := map[string]int{"Hard": 0, "Answered": 1, "Session": 1, "Record": 1, "Lens": 2, "Review": 2}
	var n [3]int
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for line := range strings.SplitSeq(string(b), "\n") {
			if w, ok := strings.CutPrefix(line, "    strength: "); ok {
				n[to[w]]++
			}
		}
	}
	return n
}

// literalRules counts the rules written in the rule files of a v2 folder, by
// reading the text and none of the migration.
func literalRules(t *testing.T, dir string) int {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*", "*", "rules", "*.yaml"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no rule files below %s: %v", dir, err)
	}
	n := 0
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		n += strings.Count(string(b), "\n  - id: ")
	}
	return n
}

// TestMigrateTheOwnersRulebookCopyCarriesEveryRule: a copy of the owner's
// rulebook migrates with --apply, the rules carried equal the rules written,
// a second run ends done with nothing to do, the plan finds no prose to
// reword, validate ends done holding every rule carried from the prose lint
// until it is reworded, and the live rulebook is untouched.
func TestMigrateTheOwnersRulebookCopyCarriesEveryRule(t *testing.T) {
	t.Parallel()
	live := oldBook() // the owner's format 2 rulebook, frozen: the live one is format 3 now
	before := checksum(t, live)
	dir := newFolder(t)
	copyTree(t, live, dir)
	copied := checksum(t, dir)
	rules := literalRules(t, dir)

	plan := migrateRun(t, dir, "80", migrateFlags()...)
	if plan.code != 6 {
		t.Fatalf("dry run: exit %d, want 6\n%s%s", plan.code, plan.stdout, plan.stderr)
	}
	if checksum(t, dir) != copied {
		t.Fatal("the dry run changed the copy")
	}
	m := carriedLine.FindStringSubmatch(plan.stdout)
	if m == nil {
		t.Fatalf("no carry-over line in the plan\n%s", plan.stdout)
	}
	if count(m[1]) != strconv.Itoa(rules) {
		t.Errorf("carried %s rules, the source writes %d", m[1], rules)
	}
	prose := proseLine.FindStringSubmatch(plan.stdout)
	if prose == nil {
		t.Fatalf("the plan names no prose count\n%s", plan.stdout)
	}
	// The strength mapping is printed once, with the count of each, and the
	// counts are those of the words written.
	if s, want := strengthsAt.FindAllStringSubmatch(flat(plan.stdout), -1), literalStrengths(t, dir); len(s) != 1 ||
		count(s[0][1]) != strconv.Itoa(want[0]) || count(s[0][2]) != strconv.Itoa(want[1]) || count(s[0][3]) != strconv.Itoa(want[2]) {
		t.Errorf("strengths %v, want one line of must %d, should %d, may %d\n%s", s, want[0], want[1], want[2], plan.stdout)
	}
	// Each old relation went to relates or conflicts, the plan says so once with
	// the count of each word, and the counts are those of the words written.
	if r, want := relationsAt.FindAllStringSubmatch(flat(plan.stdout), -1), literalRelations(t, dir); len(r) != 1 ||
		count(r[0][1]) != strconv.Itoa(want[0]) || count(r[0][2]) != strconv.Itoa(want[1]) || count(r[0][3]) != strconv.Itoa(want[2]) || count(r[0][4]) != strconv.Itoa(want[3]) {
		t.Errorf("relations %v, want one line of see %d, refines %d, conflicts %d and %d winners\n%s", r, want[0], want[1], want[2], want[3], plan.stdout)
	}
	// One side file holds every value with no field, and the plan names it with
	// its line count (items 2 to 4). The copy is the rulebook's folder, so the
	// file is at its root.
	if p := parkedAt.FindStringSubmatch(flat(plan.stdout)); p == nil || p[2] != "parked.yaml" || count(p[1]) == "0" {
		t.Errorf("the plan names no parked file with its lines: %v\n%s", p, plan.stdout)
	}
	// The remainder in no file is exactly what stays out of
	// the parked file: two kinds' instances, the parts file's waivers and the two
	// trial sources, whose kind has none.
	var left []string
	for _, n := range notCarried.FindAllStringSubmatch(plan.stdout, -1) {
		left = append(left, n[1])
	}
	if got := strings.Join(left, ","); got != "kind cli instances,kind web-app instances,parts.yaml waivers,source gragas-926-linux-spike,source gragas-926-windows-spike" {
		t.Errorf("not carried: %s, want the five of the remainder\n%s", got, plan.stdout)
	}

	if r := migrateRun(t, dir, "80", append(migrateFlags(), "--apply")...); r.code != 0 {
		t.Fatalf("migrate --apply: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	// The one conflict the owner declared is written as the relation the
	// reader takes, so validate sees the pair as declared.
	if b, err := os.ReadFile(filepath.Join(dir, "program", "cli", "rules", "cycle.yaml")); err != nil ||
		!regexp.MustCompile(`conflicts:\s+- cli\.commands\.15\s+- cli\.look\.8`).Match(b) {
		t.Errorf("cli.cycle.1 does not hold its two conflicts as the relation (%v)", err)
	}
	again := migrateRun(t, dir, "80", append(migrateFlags(), "--apply")...)
	if again.code != 0 || !strings.Contains(again.stdout, "nothing to do") {
		t.Errorf("a second migrate: exit %d\n%s%s", again.code, again.stdout, again.stderr)
	}
	v := validateRun(t, dir, "80")
	held := heldLine.FindStringSubmatch(v.stdout)
	if v.code != 0 || held == nil || count(held[1]) != count(m[1]) || count(prose[1]) != "0" {
		t.Errorf("validate: exit %d, held %v; want done with all %s rules carried held, and the plan said %s prose findings, want 0\n%s", v.code, held, m[1], prose[1], v.stdout)
	}
	if checksum(t, live) != before {
		t.Error("the live rulebook changed")
	}
}

// v2Folder is the planted v2 rulebook of testdata/v2 in a git folder of its own.
func v2Folder(t *testing.T) string {
	t.Helper()
	dir := newFolder(t)
	copyTree(t, filepath.Join(moduleRoot(), "internal", "journeys", "testdata", "v2"), dir)
	return dir
}

// TestMigrateTheFixtureListsWhatItLeavesAndValidatesClean: the planted v2
// rulebook has one facet, a rule with a binding, a source kind and a slug the
// format has no home for, and an id no rule holds. Expected counts are literals.
func TestMigrateTheFixtureListsWhatItLeavesAndValidatesClean(t *testing.T) {
	t.Parallel()
	dir := v2Folder(t)
	plan := migrateRun(t, dir, "80", migrateFlags()...)
	m := carriedLine.FindStringSubmatch(plan.stdout)
	if plan.code != 6 || m == nil {
		t.Fatalf("dry run: exit %d\n%s%s", plan.code, plan.stdout, plan.stderr)
	}
	want := []string{"2", "1", "1", "1", "2"}
	for i, w := range want {
		if m[i+1] != w {
			t.Errorf("carried[%d] = %s, want %s\n%s", i, m[i+1], w, plan.stdout)
		}
	}
	for _, s := range []string{"facet no-identity became a kind", "strengths: Hard to must, 1 rule; Answered, Session and Record to should, 1 rule;",
		"parked: ", " lines in parked.yaml,", "not carried: source a-poster", "not carried: id arch.gone.1"} {
		if !strings.Contains(flat(plan.stdout), s) {
			t.Errorf("the plan lacks %q\n%s", s, plan.stdout)
		}
	}
	if r := migrateRun(t, dir, "80", append(migrateFlags(), "--apply")...); r.code != 0 {
		t.Fatalf("migrate --apply: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	if v := validateRun(t, dir, "80"); v.code != 0 {
		t.Errorf("validate over the result: exit %d\n%s%s", v.code, v.stdout, v.stderr)
	}
	if again := migrateRun(t, dir, "80", append(migrateFlags(), "--apply")...); again.code != 0 || !strings.Contains(again.stdout, "nothing to do") {
		t.Errorf("a second migrate: exit %d\n%s%s", again.code, again.stdout, again.stderr)
	}
}

// TestMigrateGoldens: the plan and the apply over the planted v2 rulebook at
// 80, 60 and 40 columns, and the carry-over report as JSON. Risk: a carry-over
// report that moves unseen (tests.generated.5).
func TestMigrateGoldens(t *testing.T) {
	t.Parallel()
	for _, cols := range []string{"80", "60", "40"} {
		t.Run(cols, func(t *testing.T) {
			plan := migrateRun(t, v2Folder(t), cols, migrateFlags()...)
			uitest.Golden(t, "migrate-declined-"+cols, runMasked(transcript(append([]string{"migrate"}, migrateFlags()...), plan)))
			args := append(migrateFlags(), "--apply")
			done := migrateRun(t, v2Folder(t), cols, args...)
			uitest.Golden(t, "migrate-done-"+cols, runMasked(transcript(append([]string{"migrate"}, args...), done)))
		})
	}
	j := migrateRun(t, v2Folder(t), "80", append(migrateFlags(), "--json")...)
	uitest.Golden(t, "json-migrate-declined", j.stdout)
}

// shellWords splits a line as a POSIX shell does for bare words, backslashes
// and single quotes, which is all the printed lines use.
func shellWords(line string) []string {
	var out []string
	var cur strings.Builder
	in, quoted := false, false
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case quoted && c == '\'':
			quoted = false
		case quoted:
			cur.WriteByte(c)
		case c == '\'':
			quoted, in = true, true
		case c == '\\' && i+1 < len(line):
			i++
			cur.WriteByte(line[i])
			in = true
		case c == ' ':
			if in {
				out = append(out, cur.String())
				cur.Reset()
				in = false
			}
		default:
			cur.WriteByte(c)
			in = true
		}
	}
	if in {
		out = append(out, cur.String())
	}
	return out
}

// TestMigrateTheApplyLineItPrintsMakesTheSameRulebook: a dry run with a
// starting version, a licence and an owner whose name holds a space ends on an
// apply line; that line, run as printed, writes the identity the plan was made
// for. Risk: an apply line that drops a flag, so the history keeps a version
// nobody asked for.
func TestMigrateTheApplyLineItPrintsMakesTheSameRulebook(t *testing.T) {
	t.Parallel()
	dir := v2Folder(t)
	args := []string{"--name", "demo", "--version", "2.3.4", "--licence", "CC0-1.0", "--owner", "A B", "--owner", "team-a", "--json"}
	plan := migrateRun(t, dir, "80", args...)
	var doc struct {
		Next string `json:"next"`
	}
	if err := json.Unmarshal([]byte(plan.stdout), &doc); err != nil || doc.Next == "" {
		t.Fatalf("no next line in the plan: %v\n%s", err, plan.stdout)
	}
	words := shellWords(doc.Next)
	if len(words) < 3 || words[0] != "gragas" || words[1] != "migrate" {
		t.Fatalf("the next line is %q", doc.Next)
	}
	for _, want := range []string{"2.3.4", "CC0-1.0", "A B", "team-a", "--apply"} {
		if !slices.Contains(words, want) {
			t.Errorf("the apply line %q lacks %q", doc.Next, want)
		}
	}
	if r := migrateRun(t, dir, "80", words[2:]...); r.code != 0 {
		t.Fatalf("the printed line: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	b, err := os.ReadFile(filepath.Join(dir, "rulebook.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"2.3.4", "CC0-1.0", "A B", "team-a"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("rulebook.yaml lacks %q\n%s", want, b)
		}
	}
}

// TestMigrateWritesNothingWhenTheResultDoesNotReadBackClean: a v2 rule file
// that holds an id twice migrates to a rulebook the loader refuses; the run
// ends with nothing written, no history file included. Risk: a read-back that
// lets a finding through, so a broken rulebook is written.
func TestMigrateWritesNothingWhenTheResultDoesNotReadBackClean(t *testing.T) {
	t.Parallel()
	dir := v2Folder(t)
	file := filepath.Join(dir, "base", "arch", "rules", "layers.yaml")
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	dup := "\n  - id: arch.layers.1\n    slug: again\n    since: 0.1.0\n    rule: Break every cycle again.\n    strength: Hard\n    status: proposed\n"
	if err := os.WriteFile(file, append(b, dup...), 0o644); err != nil {
		t.Fatal(err)
	}
	before := checksum(t, dir)
	// The registry row of migrate (spec 15.1) says not ready, exit 4.
	for _, args := range [][]string{migrateFlags(), append(migrateFlags(), "--apply")} {
		r := migrateRun(t, dir, "200", args...)
		if r.code != 4 || !strings.Contains(r.stdout+r.stderr, "not ready") || !strings.Contains(r.stdout+r.stderr, "does not read back clean") {
			t.Errorf("migrate %v: exit %d, want not ready (4) naming the read-back\n%s%s", args, r.code, r.stdout, r.stderr)
		}
	}
	j := migrateRun(t, dir, "200", append(migrateFlags(), "--json")...)
	var doc struct {
		Ended string `json:"ended"`
		Error *struct {
			Kind string `json:"kind"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(j.stdout), &doc); err != nil || j.code != 4 || doc.Ended != "not ready" || doc.Error == nil || doc.Error.Kind != "not-ready" {
		t.Errorf("migrate --json: exit %d, document %+v (%v), want not ready (4)\n%s", j.code, doc, err, j.stdout)
	}
	if checksum(t, dir) != before {
		t.Error("the folder changed")
	}
	if _, err := os.Stat(filepath.Join(dir, "history.jsonl")); err == nil {
		t.Error("a history file was written")
	}
}
