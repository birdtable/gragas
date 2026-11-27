package journeys

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/birdtable/go-console/uitest"
)

// historyRepo is a rulebook repository whose history is the named file of
// testdata/history, or none: the rulebook itself is the same each time.
func historyRepo(t *testing.T, history string) string {
	t.Helper()
	dir := t.TempDir()
	copyTree(t, filepath.Join(moduleRoot(), "internal/journeys/testdata/history/rulebook"), dir)
	if history != "" {
		copyFile(t, filepath.Join(moduleRoot(), "internal/journeys/testdata/history", history), filepath.Join(dir, "history.jsonl"))
	}
	return dir
}

// TestTheHistoryGoldens: history on no history, a long one, the long one
// filtered by an id the rule had before it was renumbered, and a damaged one,
// as a pipe at 80, 60 and 40 columns, each against its golden, written only
// with -update by a person who reads the diff (tests.generated.5, cli.look.8).
// Risk: a listing that changes shape unseen, or an alias that stops resolving.
func TestTheHistoryGoldens(t *testing.T) {
	t.Parallel()
	for _, cols := range []string{"80", "60", "40"} {
		for _, tc := range []struct {
			name    string
			history string
			args    []string
		}{
			{"history-empty", "", []string{"history"}},
			{"history-long", "history-long.jsonl", []string{"history"}},
			{"history-alias", "history-long.jsonl", []string{"history", "go.errors.1"}},
			{"history-damaged", "history-damaged.jsonl", []string{"history"}},
			{"history-help", "", []string{"history", "--help"}},
		} {
			t.Run(tc.name+"-"+cols, func(t *testing.T) {
				env, _ := folders(t)
				env["COLUMNS"] = cols
				r := run(t, historyRepo(t, tc.history), env, tc.args...)
				if r.code != 0 {
					t.Fatalf("exit %d\n%s%s", r.code, r.stdout, r.stderr)
				}
				got := "$ gragas " + strings.Join(tc.args, " ") + "\n--- stdout\n" + r.stdout + "--- stderr\n" + r.stderr
				uitest.Golden(t, tc.name+"-"+cols, got)
			})
		}
	}
}

// TestHistoryJSONIsOneDocumentWithTheSameFieldsOnEveryRow: --json is one
// document with a schema number; filtered by an alias it keeps the rows the
// rule had under either id and says how many the file holds (cli.commands.24).
func TestHistoryJSONIsOneDocumentWithTheSameFieldsOnEveryRow(t *testing.T) {
	t.Parallel()
	env, _ := folders(t)
	r := run(t, historyRepo(t, "history-long.jsonl"), env, "history", "go.errors.1", "--json")
	if r.code != 0 {
		t.Fatalf("exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	var doc struct {
		Schema  int              `json:"schema"`
		Total   int              `json:"total"`
		Next    int              `json:"next"`
		Entries []map[string]any `json:"entries"`
	}
	if err := json.Unmarshal([]byte(r.stdout), &doc); err != nil {
		t.Fatalf("stdout is not one JSON document: %v\n%s", err, r.stdout)
	}
	if doc.Schema != 1 || doc.Total != 31 || doc.Next != 31 || len(doc.Entries) == 0 || len(doc.Entries) >= 30 {
		t.Fatalf("schema %d total %d next %d entries %d", doc.Schema, doc.Total, doc.Next, len(doc.Entries))
	}
	for _, e := range doc.Entries {
		for _, k := range []string{"number", "previous", "kind", "entity", "before", "after", "who", "when", "plan", "decision"} {
			if _, ok := e[k]; !ok {
				t.Fatalf("a row has no %q: %v", k, e)
			}
		}
	}
	uitest.Golden(t, "json-history-alias", r.stdout)
}

// TestHistoryOutsideARulebookEndsNotReady: a folder with no rulebook has no
// history, and history says so on exit 4 and changes nothing.
func TestHistoryOutsideARulebookEndsNotReady(t *testing.T) {
	t.Parallel()
	env, _ := folders(t)
	r := run(t, t.TempDir(), env, "history")
	if r.code != 4 || !strings.Contains(r.stdout+r.stderr, "not ready") {
		t.Fatalf("exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
}
