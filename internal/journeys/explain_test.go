package journeys

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/birdtable/go-console/uitest"
)

// explainRules is coverRules with an alias on the first rule, which a rule
// renumbered keeps (S62).
func explainRules() string {
	return strings.Replace(coverRules, "    strength: must\n", "    aliases: [go.errors.9]\n    strength: must\n", 1)
}

func explainRun(t *testing.T, dir string, args ...string) result {
	t.Helper()
	env, _ := folders(t)
	env["COLUMNS"] = "80"
	env["GRAGAS_NAMES_FILE"] = ""
	return run(t, dir, env, args...)
}

// TestTheExplainAndResolveGoldens: explain of a rule by its id and by an alias, of an
// entry a rule holds and of one no rule holds, of an id no rule has, and
// resolve over a rulebook of one layer, each at 80 columns against its golden
// (cli.look.8). Risk: an alias that does not reach its rule, an unmapped entry
// that does not name its tool, or a resolve with no layer.
func TestTheExplainAndResolveGoldens(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		args  []string
		code  int
		wants string
	}{
		{"explain-rule", []string{"explain", "go.errors.1"}, 0, "go.errors.1:"},
		{"explain-alias", []string{"explain", "go.errors.9"}, 0, "go.errors.9 is now go.errors.1"},
		{"explain-entry", []string{"explain", "golangci-lint", "errorlint"}, 0, "go.errors.1"},
		{"explain-unmapped", []string{"explain", "golangci-lint", "unused"}, 0, "unmapped: no rule explains this entry (golangci-lint)"},
		{"explain-no-rule", []string{"explain", "go.errors.77"}, 2, "no rule has the id"},
		{"resolve-one-layer", []string{"resolve"}, 0, "2 entries in 1 layer"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := rulebook(t, map[string]string{"language/go/rules/errors.yaml": explainRules()})
			r := explainRun(t, dir, tc.args...)
			if r.code != tc.code || !strings.Contains(r.stdout+r.stderr, tc.wants) {
				t.Errorf("exit %d, want %d naming %q\n%s%s", r.code, tc.code, tc.wants, r.stdout, r.stderr)
			}
			uitest.Golden(t, tc.name+"-80", transcript(tc.args, r))
		})
	}
}

// TestExplainAndResolveAreOneDocumentEachUnderJSON.
func TestExplainAndResolveAreOneDocumentEachUnderJSON(t *testing.T) {
	t.Parallel()
	dir := rulebook(t, map[string]string{"language/go/rules/errors.yaml": explainRules()})
	var e struct {
		Contract, Command, Word string
		Alias                   string
		Rules                   []struct{ Rule, Standing string }
		Unmapped                bool
	}
	r := explainRun(t, dir, "explain", "go.errors.9", "--json")
	if err := json.Unmarshal([]byte(r.stdout), &e); err != nil || r.code != 0 || e.Command != "explain" || e.Alias != "go.errors.1" || len(e.Rules) != 1 || e.Rules[0].Rule != "go.errors.1" {
		t.Errorf("explain: %v, exit %d, %+v\n%s", err, r.code, e, r.stdout)
	}
	var d struct {
		Command string
		Layers  []string
		Entries []struct{ Rule, Layer, How string }
	}
	r = explainRun(t, dir, "resolve", "--json")
	if err := json.Unmarshal([]byte(r.stdout), &d); err != nil || r.code != 0 || d.Command != "resolve" || len(d.Layers) != 1 || len(d.Entries) != 2 || d.Entries[0].How != "set" {
		t.Errorf("resolve: %v, exit %d, %+v\n%s", err, r.code, d, r.stdout)
	}
}

// TestAnOverlayWhoseParentIsNotFetchedEndsNotReadyNamingThePin: resolve reads the
// parent's release from the cache and fetches nothing (A10), so with none there
// it ends not ready (exit 4) naming the pin; with the stand-in in the cache it
// resolves, and each entry names the layer it came from (A50).
func TestAnOverlayWhoseParentIsNotFetchedEndsNotReadyNamingThePin(t *testing.T) {
	t.Parallel()
	dir := overlayRepo(t)
	env, _ := folders(t)
	env["COLUMNS"] = "80"
	env["GRAGAS_NAMES_FILE"] = ""
	r := run(t, dir, env, "resolve")
	if r.code != 4 || !strings.Contains(r.stdout+r.stderr, "parent not fetched: fixture-parent 0.9.0") {
		t.Fatalf("resolve: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	uitest.Golden(t, "resolve-parent-not-fetched-80", transcript([]string{"resolve"}, r))
	fetchParent(t, env)
	r = run(t, dir, env, "resolve")
	if r.code != 0 || !strings.Contains(r.stdout, "layers, root first: fixture-parent") {
		t.Fatalf("resolve with the parent: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	uitest.Golden(t, "resolve-overlay-80", transcript([]string{"resolve"}, r))
}
