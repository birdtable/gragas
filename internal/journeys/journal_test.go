package journeys

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/birdtable/go-console/uitest"
)

func sum(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func digestText(s string) string { return "sha256:" + sum(s) }

// journalWorld is a working folder and the state folder its journal is kept
// in, written by hand in the journal's own format: the binary reads what the
// write path writes, and the lines are the ones it would have made. That
// format is runlog's (go-run v0.8.0): each record an entry line of its run,
// its value the record, beside the run's record line under its id.
// A line of the older form, one to a record under its run with
// no id, is refused by the engine, so no plant here writes one.
type journalWorld struct {
	t       *testing.T
	repo    string
	state   string
	env     map[string]string
	root    string   // the digest of the folder, as the journal names it
	runs    []string // the runs, in the order their first record came
	entries map[string][]json.RawMessage
}

func newJournalWorld(t *testing.T) *journalWorld {
	t.Helper()
	repo := fixture(t)
	real, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatal(err)
	}
	env, state := folders(t)
	return &journalWorld{t: t, repo: repo, state: state, env: env, root: sum(filepath.ToSlash(real))}
}

// line adds one record to run's list: the value of the entry line the
// engine's append writes, with no run of its own, since the entry line names
// it.
func (w *journalWorld) line(run, kind string, at int, detail string) {
	l := map[string]any{"v": 1, "kind": kind, "at": at, "command": "gragas gen --apply", "plan": digestText("plan " + run), "root": w.root, "engine": "v0-test"}
	if detail != "" {
		l["detail"] = json.RawMessage(detail)
	}
	b, err := json.Marshal(l)
	if err != nil {
		w.t.Fatal(err)
	}
	if w.entries == nil {
		w.entries = map[string][]json.RawMessage{}
	}
	if _, ok := w.entries[run]; !ok {
		w.runs = append(w.runs, run)
	}
	w.entries[run] = append(w.entries[run], b)
}

// runLine is run's lines in runlog's form.
func (w *journalWorld) runLine(run string) string {
	return runlogLine(w.t, run, w.entries[run]...)
}

// runlogLine is the lines of run in a journal in runlog's form (go-run
// v0.8.0), as the engine's append leaves them, joined by newlines with no
// last one: each record an entry line of the run, by runlog's Entry, its
// value the record; after the first, the run's record line, its id the run,
// started at that record; and after a close, the record line again, finished
// at it, as runlog's Finish appends it.
func runlogLine(t *testing.T, run string, entries ...json.RawMessage) string {
	t.Helper()
	var out []string
	add := func(l map[string]any) {
		b, err := json.Marshal(l)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, string(b))
	}
	var started time.Time
	for i, e := range entries {
		var head struct {
			Kind string `json:"kind"`
			At   int64  `json:"at"`
		}
		if err := json.Unmarshal(e, &head); err != nil {
			t.Fatal(err)
		}
		at := time.Unix(head.At, 0).UTC()
		add(map[string]any{"entry": run, "time": at, "value": e})
		record := map[string]any{"id": run, "argv": []string{"gragas", "gen", "--apply"}, "started": started, "code": 0, "changes": []string{}}
		switch {
		case i == 0:
			started = at
			record["started"] = at
			add(record)
		case head.Kind == "close":
			record["finished"] = at
			add(record)
		}
	}
	return strings.Join(out, "\n")
}

// copy records the copy of path a run kept, and stores its bytes.
func (w *journalWorld) copy(run, path, before string, existed bool, at int) {
	d := ""
	if existed {
		d = digestText(before)
		put(w.t, filepath.Join(w.state, w.root+".copies", run), map[string]string{sum(path): before})
	}
	detail, err := json.Marshal(map[string]any{"path": path, "existed": existed, "before": d})
	if err != nil {
		w.t.Fatal(err)
	}
	w.line(run, "copy", at, string(detail))
}

func (w *journalWorld) closed(run, outcome, path, before, after string, at int) {
	files := []map[string]string{}
	if path != "" {
		files = append(files, map[string]string{"path": path, "before": digestText(before), "after": digestText(after)})
	}
	detail, err := json.Marshal(map[string]any{"outcome": outcome, "file": "", "files": files})
	if err != nil {
		w.t.Fatal(err)
	}
	w.line(run, "close", at, string(detail))
}

// write puts the journal in the state folder and the files in the folder.
func (w *journalWorld) write(files map[string]string) {
	var lines []string
	for _, run := range w.runs {
		lines = append(lines, w.runLine(run))
	}
	put(w.t, w.state, map[string]string{w.root + ".journal": strings.Join(lines, "\n") + "\n"})
	put(w.t, w.repo, files)
}

// closedAndOpen is r1, closed, that replaced a.txt "old" with "new", r2, rolled
// back, that made b.txt, and r3, interrupted after it kept a copy of c.txt and
// wrote c as the content below.
func closedAndOpen(w *journalWorld, c string) {
	w.line("r1", "intent", 1779000000, "")
	w.copy("r1", "a.txt", "old", true, 1779000001)
	w.closed("r1", "done", "a.txt", "old", "new", 1779000010)
	w.line("r2", "intent", 1779000100, "")
	w.copy("r2", "b.txt", "", false, 1779000101)
	w.line("r2", "close", 1779000110, `{"outcome":"rolled back","file":"","files":[]}`)
	w.line("r3", "intent", 1779000200, "")
	w.copy("r3", "c.txt", "kept", true, 1779000201)
	// The roll-back puts a file back only if it holds its before or what a step of
	// the run left, so the run's one step is the content the tests plant.
	w.line("r3", "step", 1779000202, fmt.Sprintf(`{"path":"c.txt","op":"write","after":%q}`, digestText(c)))
	w.write(map[string]string{"a.txt": "new", "c.txt": c})
}

// scrubbed hides what moves with the machine: the folder's digest and the
// state folder's path, as given and as the product names it, every link on it
// followed (a root is named by its link-free path, Tree.Open), whichever the
// test's temporary folder is. The resolved spelling is masked first, since
// the given one may be the tail of it. A word the product quoted only because
// the state path is not one a shell reads bare is read as the bare word it is
// once masked; where the state path is bare, nothing is unquoted, so the
// goldens still hold that quote leaves a bare path bare.
func (w *journalWorld) scrubbed(s string) string {
	s = strings.ReplaceAll(s, w.root, "ROOT")
	s = strings.ReplaceAll(s, w.root[:12], "ROOT")
	// The path with its links followed goes first: the path as given is the
	// end of it, and would leave the front of it in the text.
	// The binary prints the state folder with its links followed when it is
	// reached through one (a macOS temp folder is under /private); the folder
	// is made by the time a transcript is read, or has no links to follow.
	real, err := filepath.EvalSymlinks(w.state)
	if err != nil {
		real = w.state
	}
	states := []string{real, w.state}
	for _, p := range states {
		s = strings.ReplaceAll(s, p, "$STATE")
		// In a --json document the path is a JSON string, so a backslash in
		// it is written as two; mask that spelling too. Where the path holds
		// none, it is the same text and the line above has already masked it.
		if esc, err := json.Marshal(p); err == nil && len(esc) > 2 {
			s = strings.ReplaceAll(s, string(esc[1:len(esc)-1]), "$STATE")
		}
	}
	// A masked path keeps the separators of the system it was made on; the
	// goldens hold forward slashes, so put them back before the quote is read.
	s = slashMasked(s)
	// Either spelling the product may print can be one its quote wraps.
	if !slices.ContainsFunc(states, func(p string) bool { return !bareWord(p) }) {
		return s
	}
	return unquoteMasked(s)
}

// statePath is a $STATE run up to a space or a quote: the state mask reached it,
// so it is a path, and its backslashes are a join's separators on the system
// that made it.
var statePath = regexp.MustCompile(`\$STATE[^\s'"]*`)

// slashMasked is s with every masked path's separators the forward slashes the
// goldens hold: a backslash written once, as a plain path, or twice, as a JSON
// string's escape. On a system whose paths are slashes already it changes
// nothing. It folds a backslash the product wrote into a masked path too, so on
// a slash system the goldens cannot catch a join that wrongly used one; that is
// acceptable because the product joins with path and filepath, never by hand
// (on).
func slashMasked(s string) string {
	return statePath.ReplaceAllStringFunc(s, func(m string) string {
		return strings.ReplaceAll(strings.ReplaceAll(m, `\\`, "/"), `\`, "/")
	})
}

// bareWord is whether every byte of s is one the product's quote leaves bare.
func bareWord(s string) bool {
	for _, c := range []byte(s) {
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && strings.IndexByte(bareSafe, c) < 0 {
			return false
		}
	}
	return s != ""
}

// maskedWord is a whole single-quoted word the state mask reached: it starts
// and ends at a space or the text's edge, so the escape of a quote inside
// a longer word is never cut.
var maskedWord = regexp.MustCompile(`(^|\s)'([^'\s]*\$STATE[^'\s]*)'(\s|$)`)

// bareSafe is every byte the product's quote leaves a word bare for: a letter,
// a digit, or one of _-./,:+=@%.
const bareSafe = "_-./,:+=@%"

// unquoteMasked reads a word the product quoted for the machine's state path,
// which on Windows holds a drive colon and backslashes and so is quoted, as the
// product prints the same word on a POSIX path: bare, once the mask has
// replaced the path. A word that stays unsafe after the mask keeps its quotes.
func unquoteMasked(s string) string {
	for {
		out := maskedWord.ReplaceAllStringFunc(s, func(m string) string {
			p := maskedWord.FindStringSubmatch(m)
			if !bareWord(strings.ReplaceAll(p[2], "$", "")) {
				return m
			}
			return p[1] + p[2] + p[3]
		})
		if out == s {
			return s
		}
		s = out
	}
}

func (w *journalWorld) transcript(name, cols string, args ...string) string {
	w.t.Helper()
	env := map[string]string{"COLUMNS": cols}
	maps.Copy(env, w.env)
	r := run(w.t, w.repo, env, args...)
	return w.scrubbed("$ gragas " + strings.Join(args, " ") + "\n--- exit " + string(rune('0'+r.code)) + "\n--- stdout\n" + r.stdout + "--- stderr\n" + r.stderr)
}

// TestTheJournalGoldens: list over no journal and over a closed, a rolled-back
// and an open run; recover without and with --apply; restore done and refused;
// and the three helps, each at 80, 60 and 40 columns against its golden,
// written only with -update by a person who reads the diff (cli.look.8).
func TestTheJournalGoldens(t *testing.T) {
	t.Parallel()
	for _, cols := range []string{"80", "60", "40"} {
		cases := []struct {
			name  string
			setup func(w *journalWorld)
			args  []string
		}{
			{"journal-list-empty", func(*journalWorld) {}, []string{"journal", "list"}},
			{"journal-list", func(w *journalWorld) { closedAndOpen(w, "half") }, []string{"journal", "list"}},
			{"journal-recover-dry", func(w *journalWorld) { closedAndOpen(w, "half") }, []string{"journal", "recover"}},
			{"journal-recover-apply", func(w *journalWorld) { closedAndOpen(w, "half") }, []string{"journal", "recover", "--apply"}},
			{"journal-recover-nothing", func(*journalWorld) {}, []string{"journal", "recover"}},
			{"journal-restore", func(w *journalWorld) { closedAndOpen(w, "half") }, []string{"journal", "restore", "r1"}},
			{"journal-restore-refused", func(w *journalWorld) {
				closedAndOpen(w, "half")
				w.line("r4", "intent", 1779000300, "")
				w.copy("r4", "a.txt", "new", true, 1779000301)
				w.closed("r4", "done", "a.txt", "new", "newer", 1779000310)
				w.write(map[string]string{"a.txt": "newer"})
			}, []string{"journal", "restore", "r1"}},
			{"journal-restore-open", func(w *journalWorld) { closedAndOpen(w, "half") }, []string{"journal", "restore", "r3"}},
			{"journal-help", func(*journalWorld) {}, []string{"journal", "--help"}},
			{"journal-recover-help", func(*journalWorld) {}, []string{"journal", "recover", "--help"}},
			{"journal-restore-help", func(*journalWorld) {}, []string{"journal", "restore", "--help"}},
			{"cache-clear-help", func(*journalWorld) {}, []string{"cache", "clear", "--help"}},
		}
		for _, tc := range cases {
			t.Run(tc.name+"-"+cols, func(t *testing.T) {
				w := newJournalWorld(t)
				tc.setup(w)
				uitest.Golden(t, tc.name+"-"+cols, w.transcript(tc.name, cols, tc.args...))
			})
		}
	}
}

// TestTheJournalJSONDocuments: --json is one document with a schema number,
// the folder's digest and none of its path, for list, restore and a recover
// that finds nothing open.
func TestTheJournalJSONDocuments(t *testing.T) {
	t.Parallel()
	for name, args := range map[string][]string{
		"json-journal-list":    {"journal", "list", "--json"},
		"json-journal-restore": {"journal", "restore", "r1", "--json"},
		"json-journal-recover": {"journal", "recover", "--apply", "--json"},
	} {
		t.Run(name, func(t *testing.T) {
			w := newJournalWorld(t)
			closedAndOpen(w, "half")
			r := run(t, w.repo, w.env, args...)
			if r.code != 0 {
				t.Fatalf("exit %d\n%s%s", r.code, r.stdout, r.stderr)
			}
			var doc map[string]any
			if err := json.Unmarshal([]byte(r.stdout), &doc); err != nil || doc["schema"] != float64(1) {
				t.Fatalf("not one document with a schema number: %v\n%s", err, r.stdout)
			}
			if strings.Contains(r.stdout, w.repo) {
				t.Errorf("the document names the folder:\n%s", r.stdout)
			}
			if name != "json-journal-recover" { // its run id is made fresh each time
				uitest.Golden(t, name, w.scrubbed(r.stdout))
			}
		})
	}
}

// TestThePrintedLinesPutTheFilesBack: the line restore prints names the kept
// copy and the file as the --json files say them, and doing that step puts
// a.txt back to its copy. This holds the name the restore gives a copy to the
// name the journal stores it under. No shell is started: the line is split as
// a POSIX shell splits it, and held to `cp -- <copy> a.txt`.
// Risk: a line that names another copy, a wrong path or a broken quote, which
// a person pastes and which puts the wrong bytes back.
func TestThePrintedLinesPutTheFilesBack(t *testing.T) {
	t.Parallel()
	w := newJournalWorld(t)
	closedAndOpen(w, "half")
	r := run(t, w.repo, w.env, "journal", "restore", "r1", "--json")
	var doc struct {
		Lines []string
		Files []struct{ Op, Path, From string }
	}
	if err := json.Unmarshal([]byte(r.stdout), &doc); err != nil || len(doc.Lines) != 1 || len(doc.Files) != 1 {
		t.Fatalf("restore said %v, %v, %v\n%s", doc.Lines, doc.Files, err, r.stdout)
	}
	f := doc.Files[0]
	if f.Op != "restore" || f.Path != "a.txt" {
		t.Fatalf("the step is %+v, want a restore of a.txt", f)
	}
	// The line, as a shell would read it, is the step the file says as data.
	rest, ok := strings.CutPrefix(doc.Lines[0], "cp -- ")
	if !ok {
		t.Fatalf("the line %q does not start with %q", doc.Lines[0], "cp -- ")
	}
	word, ok := strings.CutSuffix(rest, " "+f.Path)
	if !ok {
		t.Fatalf("the line %q does not end with the path %q as one word", doc.Lines[0], f.Path)
	}
	if got := posixWord(word); got != f.From {
		t.Errorf("the line names the copy %q, but the file says %q (line %q)", got, f.From, doc.Lines[0])
	}
	kept, err := os.ReadFile(f.From)
	if err != nil {
		t.Fatalf("the kept copy %s: %v", f.From, err)
	}
	if err := os.WriteFile(filepath.Join(w.repo, f.Path), kept, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(w.repo, "a.txt"))
	if err != nil || string(got) != "old" {
		t.Errorf("a.txt holds %q (%v), want the copy %q", got, err, "old")
	}
}

// posixWord is w, one word of a POSIX shell's line as printed by a quoter that
// wraps a word with a special character in single quotes (a quote inside it
// closed, escaped and opened again), read back to the text it stands for; a
// word with no quote is as it is.
func posixWord(w string) string {
	if len(w) >= 2 && strings.HasPrefix(w, "'") && strings.HasSuffix(w, "'") {
		return strings.ReplaceAll(w[1:len(w)-1], `'\''`, "'")
	}
	return w
}

// TestARestoreInPowerShellOrCmdSaysEachStepInWords: rm and
// cp are no programs gragas can know read their words as written there, so
// the restore prints no rm or cp line; it says the step in words, each path
// as %q, and ends done. --json carries the step in files and no line, as
// lines are lines to type. Risk: an rm or cp line pasted where it is an alias
// or no command, or words in lines a program runs.
func TestARestoreInPowerShellOrCmdSaysEachStepInWords(t *testing.T) {
	t.Parallel()
	for _, sh := range []string{"powershell", "cmd"} {
		t.Run(sh, func(t *testing.T) {
			t.Parallel()
			w := newJournalWorld(t)
			closedAndOpen(w, "half")
			env := maps.Clone(w.env)
			env["GRAGAS_SHELL"] = sh
			r := run(t, w.repo, env, "journal", "restore", "r1")
			out := w.scrubbed(r.stdout)
			step := `copy "$STATE/ROOT.copies/r1/` + sum("a.txt") + `" over "a.txt"`
			if r.code != 0 || !strings.Contains(out, step) || strings.Contains(out, "cp ") || !strings.Contains(flat(out), "1 step put run r1 back") {
				t.Fatalf("exit %d, want 0 and the step %s in words\n%s%s", r.code, step, out, r.stderr)
			}
			r = run(t, w.repo, env, "journal", "restore", "r1", "--json")
			var doc struct {
				Lines []string
				Files []struct{ Op, Path string }
			}
			if err := json.Unmarshal([]byte(r.stdout), &doc); err != nil || r.code != 0 || doc.Lines == nil || len(doc.Lines) != 0 ||
				len(doc.Files) != 1 || doc.Files[0].Op != "restore" || doc.Files[0].Path != "a.txt" {
				t.Fatalf("--json: exit %d, %v, %+v; want the step in files and no line\n%s", r.code, err, doc, r.stdout)
			}
		})
	}
}

// TestRecoverApplyPutsAFileBackAndAFreshListShowsTheRunRolledBack: the real
// binary rolls r3 back, c.txt holds its copy, and the next list says so.
func TestRecoverApplyPutsAFileBackAndAFreshListShowsTheRunRolledBack(t *testing.T) {
	t.Parallel()
	w := newJournalWorld(t)
	closedAndOpen(w, "half")
	if r := run(t, w.repo, w.env, "journal", "recover", "--apply"); r.code != 0 {
		t.Fatalf("exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	if got, _ := os.ReadFile(filepath.Join(w.repo, "c.txt")); string(got) != "kept" {
		t.Errorf("c.txt holds %q, want the copy", got)
	}
	if r := run(t, w.repo, w.env, "journal", "list"); !strings.Contains(r.stdout, "r3") || strings.Contains(r.stdout, "open") {
		t.Errorf("the list after:\n%s", r.stdout)
	}
}
