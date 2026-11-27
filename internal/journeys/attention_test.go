package journeys

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/birdtable/go-console/uitest"
)

// attentionRepo is a copy of the named rulebook of testdata/attention.
func attentionRepo(t *testing.T, name string) string {
	t.Helper()
	dir := t.TempDir()
	copyTree(t, filepath.Join(moduleRoot(), "internal/journeys/testdata/attention", name), dir)
	return dir
}

// today is the day the binary judged by, which its clock gives and no test
// moves; a golden keeps the word instead.
var today = regexp.MustCompile(`"today": "\d{4}-\d{2}-\d{2}"`)

// TestTheAttentionGoldens: attention on a book where every rule is proposed and
// on one with an overdue review-by, as a pipe at 80, 60 and 40 columns, each
// against its golden, written only with -update by a person who reads the diff
// (tests.generated.5, cli.look.8). The dates of the fixtures are 2000 and 2999
// so the day the test runs on moves nothing. Risk: a listing that changes shape
// unseen, or an overdue rule that is not named.
func TestTheAttentionGoldens(t *testing.T) {
	t.Parallel()
	for _, cols := range []string{"80", "60", "40"} {
		for _, tc := range []struct {
			name, book string
			args       []string
		}{
			{"attention-proposed", "proposed", []string{"attention"}},
			{"attention-overdue", "overdue", []string{"attention"}},
			{"attention-help", "proposed", []string{"attention", "--help"}},
		} {
			t.Run(tc.name+"-"+cols, func(t *testing.T) {
				env, _ := folders(t)
				env["COLUMNS"] = cols
				r := run(t, attentionRepo(t, tc.book), env, tc.args...)
				if r.code != 0 {
					t.Fatalf("exit %d\n%s%s", r.code, r.stdout, r.stderr)
				}
				got := "$ gragas " + strings.Join(tc.args, " ") + "\n--- stdout\n" + r.stdout + "--- stderr\n" + r.stderr
				uitest.Golden(t, tc.name+"-"+cols, got)
			})
		}
	}
}

// TestAttentionJSONIsOneDocument: --json is one document with a schema number,
// the override rate as null with its reason, and the same fields on every item
// (cli.output.9, cli.commands.24).
func TestAttentionJSONIsOneDocument(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"proposed", "overdue"} {
		t.Run(name, func(t *testing.T) {
			env, _ := folders(t)
			r := run(t, attentionRepo(t, name), env, "attention", "--json")
			if r.code != 0 {
				t.Fatalf("exit %d\n%s%s", r.code, r.stdout, r.stderr)
			}
			var doc struct {
				Schema       int `json:"schema"`
				OverrideRate struct {
					Value  *float64 `json:"value"`
					Reason string   `json:"reason"`
				} `json:"override_rate"`
				AllProposed bool             `json:"all_proposed"`
				Items       []map[string]any `json:"items"`
			}
			if err := json.Unmarshal([]byte(r.stdout), &doc); err != nil {
				t.Fatalf("stdout is not one JSON document: %v\n%s", err, r.stdout)
			}
			if doc.Schema != 1 || doc.OverrideRate.Value != nil || doc.OverrideRate.Reason != "no batches given" || len(doc.Items) == 0 || doc.AllProposed != (name == "proposed") {
				t.Fatalf("schema %d override %+v items %d all proposed %v", doc.Schema, doc.OverrideRate, len(doc.Items), doc.AllProposed)
			}
			for _, i := range doc.Items {
				for _, k := range []string{"rule", "kind", "status", "why", "fix"} {
					if _, ok := i[k]; !ok {
						t.Fatalf("an item has no %q: %v", k, i)
					}
				}
			}
			uitest.Golden(t, "json-attention-"+name, today.ReplaceAllString(r.stdout, `"today": "DAY"`))
		})
	}
}

// TestAttentionOutsideARulebookEndsNotReady: a folder with no rulebook has
// nothing to attend to, and attention says so on exit 4.
func TestAttentionOutsideARulebookEndsNotReady(t *testing.T) {
	t.Parallel()
	env, _ := folders(t)
	r := run(t, t.TempDir(), env, "attention")
	if r.code != 4 || !strings.Contains(r.stdout+r.stderr, "not ready") {
		t.Fatalf("exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
}
