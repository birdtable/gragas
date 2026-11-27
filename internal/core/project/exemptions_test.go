package project_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/core/project"
)

func TestMain(m *testing.M) { os.Exit(configtest.Main(m)) }

// row is the one row of the file the plants below edit, moved with the check
// from internal/dev/gate's tests.
const row = `exemptions:
  - {id: exit, rule: cli.exit.1, path: ".", reason: "the spec needs exit 6", exit: "x/rulebook#34", owner: gate, expires: "2027-03-31"}
`

// today is the date every plant is judged on.
const today = "2026-11-08"

// active is the rulebook's active rules, as the old registry read them:
// old.rule.1 is retired, so it is not here.
func active() map[string]bool {
	return map[string]bool{"cli.exit.1": true, "gates.ratchets.8": true, "arch.measures.1": true}
}

// repo is the project's files an exit may name.
func repo() fstest.MapFS {
	return fstest.MapFS{"big.go": {Data: []byte("package big\n")}, "rulebook/parts.yaml": {Data: []byte("parts: []\n")}, "docs.x/a.md": {Data: []byte("a\n")}}
}

func check(data string) project.Exemptions {
	return project.CheckExemptions([]byte(data), active(), today, repo())
}

func strs(fs []project.Finding) []string {
	out := make([]string, len(fs))
	for i, f := range fs {
		out[i] = f.String()
	}
	return out
}

func fires(t *testing.T, found []project.Finding, want string) {
	t.Helper()
	if !slices.ContainsFunc(found, func(f project.Finding) bool { return strings.Contains(f.String(), want) }) {
		t.Errorf("no finding says %q; found %q", want, strs(found))
	}
}

// TestACleanFileHolds is the control every plant below is read against.
func TestACleanFileHolds(t *testing.T) {
	t.Parallel()
	if got := check(row); len(got.Findings) != 0 || got.Rows != 1 || got.NextExpiry != "2027-03-31" {
		t.Fatalf("the clean control: %+v", got)
	}
}

// TestAnExemptionExpiresAndNeedsALiveRuleAndAnArtifact: an expired row fails,
// so does one for a rule that is not active, one whose exit names no
// artifact, and one whose exit file is not there (gates.exemptions.2, .4).
func TestAnExemptionExpiresAndNeedsALiveRuleAndAnArtifact(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, from, to, says string
	}{
		{"an expired row", `expires: "2027-03-31"`, `expires: "2026-11-07"`, "exit expired on 2026-11-07"},
		{"a row that expires today holds", `expires: "2027-03-31"`, `expires: "2026-11-08"`, ""},
		{"a rule that is retired", "rule: cli.exit.1", "rule: old.rule.1", "exit is unused: old.rule.1 is not an active rule"},
		{"a rule that is not there", "rule: cli.exit.1", "rule: no.such.1", "exit is unused: no.such.1"},
		{"an exit that names no artifact", `exit: "x/rulebook#34"`, `exit: "later"`, `exit "later" names no artifact`},
		{"an exit file that is not there", `exit: "x/rulebook#34"`, `exit: "docs/gone.md"`, "its exit docs/gone.md is a file that does not exist"},
		{"an exit file that is there", `exit: "x/rulebook#34"`, `exit: "big.go"`, ""},
		{"an exit that is a folder", `exit: "x/rulebook#34"`, `exit: "docs.x"`, "its exit docs.x is a file that does not exist"},
		{"an exit file outside the project", `exit: "x/rulebook#34"`, `exit: "./../big.go"`, `exit "./../big.go" names no artifact`},
		{"an exit file through dot", `exit: "x/rulebook#34"`, `exit: "./big.go"`, `exit "./big.go" names no artifact`},
		{"an expiry that is not a date", `expires: "2027-03-31"`, `expires: soon`, `its expiry "soon" is not a date`},
		{"an expiry that is no real day", `expires: "2027-03-31"`, `expires: "2027-02-30"`, `its expiry "2027-02-30" is not a date`},
		{"a row with no expiry", `, expires: "2027-03-31"`, ``, `its expiry "" is not a date`},
		{"an unquoted date holds", `expires: "2027-03-31"`, `expires: 2027-03-31`, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			found := check(strings.Replace(row, tc.from, tc.to, 1)).Findings
			if tc.says == "" {
				if len(found) != 0 {
					t.Errorf("the control fails: %q", strs(found))
				}
				return
			}
			fires(t, found, tc.says)
		})
	}
	t.Run("a row listed twice", func(t *testing.T) {
		t.Parallel()
		fires(t, check(row+strings.SplitN(row, "\n", 2)[1]).Findings, "exemption exit is listed twice")
	})
}

// TestARowThatCannotBeReadIsAFinding: an exemptions that is a string, and a
// row that is a string beside a good one, are each a finding on their line,
// and the row beside it is still judged; an empty list holds.
// Risk: a malformed file read as no rows, so a project that runs only
// gragas check, or a gate that judges with it, passes a row it never read.
func TestARowThatCannotBeReadIsAFinding(t *testing.T) {
	t.Parallel()
	t.Run("exemptions that is a string", func(t *testing.T) {
		t.Parallel()
		got := check("exemptions: oops\n")
		if len(got.Findings) != 1 || got.Findings[0].Line != 1 || !strings.Contains(got.Findings[0].Msg, "exemptions is not a list of rows") {
			t.Errorf("found %q; want one finding on line 1", strs(got.Findings))
		}
	})
	t.Run("a row that is a string", func(t *testing.T) {
		t.Parallel()
		got := check(row + "  - just a string\n" + strings.Replace(strings.SplitN(row, "\n", 2)[1], `expires: "2027-03-31"`, `expires: "2026-11-07"`, 1))
		fires(t, got.Findings, "gate/exemptions.yaml:1: row 2 of exemptions is not a mapping")
		fires(t, got.Findings, "exit expired on 2026-11-07")
		if got.Rows != 3 {
			t.Errorf("rows = %d, want 3: a row that cannot be read is still a row", got.Rows)
		}
	})
	t.Run("an empty list holds", func(t *testing.T) {
		t.Parallel()
		if got := check("exemptions: []\n"); len(got.Findings) != 0 || got.Rows != 0 {
			t.Errorf("an empty list: %+v", got)
		}
	})
}

// TestTheFileItselfMustBeReadable: a file that is not YAML, not a mapping, or
// judged with no date is one finding of the whole file, never a pass.
func TestTheFileItselfMustBeReadable(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct{ data, today, says string }{
		"not YAML":      {"exemptions: [\n", today, "gate/exemptions.yaml: "},
		"not a mapping": {"- a\n- b\n", today, "gate/exemptions.yaml: is not a mapping"},
		"no date":       {row, "", "gate/exemptions.yaml: no date was given"},
		"a bad date":    {row, "tomorrow", "gate/exemptions.yaml: no date was given"},
		"no list":       {"other: 1\n", today, "gate/exemptions.yaml: exemptions is not a list of rows"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := project.CheckExemptions([]byte(tc.data), active(), tc.today, repo())
			if len(got.Findings) != 1 || got.Findings[0].Line != 0 || !strings.HasPrefix(got.Findings[0].String(), tc.says) {
				t.Errorf("found %q; want one finding of the file starting %q", strs(got.Findings), tc.says)
			}
		})
	}
}

// TestOnePlaceCannotStandForAnother: the file is read strictly, so a key
// written twice in a row or at the top, an anchor and its alias, a merge key
// and a tag are each one finding of the whole file that names the line, and
// no row is judged as passing. Risk: ParseYAML keeps the last of two equal
// keys, so a second expires hides an expired row and a second exemptions
// hides every row, in a project's own file that no reviewer stands over.
func TestOnePlaceCannotStandForAnother(t *testing.T) {
	t.Parallel()
	expired := strings.Replace(row, `expires: "2027-03-31"`, `expires: "2020-01-01"`, 1)
	for name, tc := range map[string]struct{ data, says string }{
		"expires written twice in a row": {strings.Replace(expired, `expires: "2020-01-01"`, `expires: "2020-01-01", expires: "2099-01-01"`, 1), "the key expires is written again"},
		"exemptions written twice":       {expired + "exemptions: []\n", "the key exemptions is written again, first at line 1"},
		"an anchor and its alias":        {strings.Replace(expired, "  - {id: exit", "  - &a {id: exit", 1) + "  - *a\n", "the anchor &a"},
		"a merge key":                    {strings.Replace(expired, "{id: exit", "{<<: {owner: gate}, id: exit", 1), "merge keys (<<) are not supported"},
		"a tag":                          {strings.Replace(expired, `expires: "2020-01-01"`, `expires: !!str "2099-01-01"`, 1), "the tag !!str is not read here"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := check(tc.data)
			if len(got.Findings) != 1 || got.Findings[0].Line != 0 || !strings.Contains(got.Findings[0].Msg, tc.says) || got.Rows != 0 || got.NextExpiry != "" {
				t.Errorf("found %q (rows %d, next %q); want one finding of the file saying %q", strs(got.Findings), got.Rows, got.NextExpiry, tc.says)
			}
		})
	}
}

// TestTheNextExpiryIsTheSoonestStillAhead: the count of rows and the soonest
// expiry not yet past, which gragas-dev's gate prints on every run
// (gates.ratchets.1).
func TestTheNextExpiryIsTheSoonestStillAhead(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		rows string
		n    int
		next string
	}{
		"the next expiry is the soonest still ahead": {
			`  - {id: a, rule: cli.exit.1, path: ".", reason: "r", exit: "x/rulebook#34", owner: gate, expires: "2027-03-31"}
  - {id: b, rule: cli.exit.1, path: ".", reason: "r", exit: "x/rulebook#34", owner: gate, expires: "2026-12-01"}
  - {id: c, rule: cli.exit.1, path: ".", reason: "r", exit: "x/rulebook#34", owner: gate, expires: "2026-11-08"}
`, 3, "2026-11-08"},
		"an expired row is a finding, not the next expiry": {
			`  - {id: a, rule: cli.exit.1, path: ".", reason: "r", exit: "x/rulebook#34", owner: gate, expires: "2027-03-31"}
  - {id: b, rule: cli.exit.1, path: ".", reason: "r", exit: "x/rulebook#34", owner: gate, expires: "2026-11-07"}
`, 2, "2027-03-31"},
		"with every row expired there is none": {
			`  - {id: a, rule: cli.exit.1, path: ".", reason: "r", exit: "x/rulebook#34", owner: gate, expires: "2026-01-01"}
`, 1, ""},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := check("exemptions:\n" + tc.rows)
			if got.Rows != tc.n || got.NextExpiry != tc.next {
				t.Errorf("rows %d, next expiry %q; want %d and %q", got.Rows, got.NextExpiry, tc.n, tc.next)
			}
		})
	}
}

// TestTheFixturesHoldWhatTheJourneysPrint: the fixture the journeys plant in
// a project holds the four findings, one each, and the clean one none. The
// rule ids are the active and retired rules of the journeys' rulebook.
func TestTheFixturesHoldWhatTheJourneysPrint(t *testing.T) {
	t.Parallel()
	live := map[string]bool{"arch.layers.1": true, "arch.layers.2": true, "arch.layers.4": true, "go.errors.1": true}
	read := func(name string) []byte {
		b, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	if got := project.CheckExemptions(read("clean.yaml"), live, today, repo()); len(got.Findings) != 0 || got.Rows != 2 {
		t.Errorf("clean.yaml: %+v", got)
	}
	got := project.CheckExemptions(read("four.yaml"), live, today, repo())
	want := []string{
		"gate/exemptions.yaml:5: expired expired on 2020-01-02: retire it or renew it with a reason",
		"gate/exemptions.yaml:6: retired is unused: arch.layers.3 is not an active rule of the rulebook",
		"gate/exemptions.yaml:8: exemption twice is listed twice",
		"gate/exemptions.yaml:9: lost-exit: its exit docs/gone.md is a file that does not exist",
	}
	if !slices.Equal(strs(got.Findings), want) {
		t.Errorf("four.yaml found\n%q\nwant\n%q", strs(got.Findings), want)
	}
}
