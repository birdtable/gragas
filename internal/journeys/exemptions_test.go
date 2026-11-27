package journeys

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/birdtable/go-console/uitest"

	testenv "github.com/birdtable/go-testkit"
)

// exemptionsAt plants the fixture of internal/core/project named by name as
// the project's gate/exemptions.yaml.
func exemptionsAt(t *testing.T, repo, name string) {
	t.Helper()
	copyFile(t, filepath.Join(moduleRoot(), "internal/core/project/testdata", name), filepath.Join(repo, "gate", "exemptions.yaml"))
}

// TestTheGoldensHoldTheExemptionsCheck: check reads gate/exemptions.yaml
// when the project has it and lists what is wrong with it beside the
// rulebook's findings, in text and in --json, each held by a golden
// (gates.exemptions.4, gates.plants.1). The fixture with four things wrong
// lists four; the clean one lists none. Its rows expire in 2020 and 2099, so
// no golden moves with the day it runs. Risk: an expired exception that check
// passes in silence.
func TestTheGoldensHoldTheExemptionsCheck(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, file string
		code       int
		ended      string
		rows       float64
		findings   int
	}{
		{"four", "four.yaml", 1, "findings", 5, 4},
		{"clean", "clean.yaml", 0, "done", 2, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo := fixture(t)
			exemptionsAt(t, repo, tc.file)
			r := run(t, repo, nil, "check")
			if r.code != tc.code {
				t.Fatalf("exit %d, want %d\n%s%s", r.code, tc.code, r.stdout, r.stderr)
			}
			uitest.Golden(t, "check-exemptions-"+tc.name, scrub("$ gragas check\n--- stdout\n"+r.stdout+"--- stderr\n"+r.stderr, ""))
			r = run(t, repo, nil, "check", "--json")
			if r.code != tc.code {
				t.Fatalf("--json: exit %d, want %d\n%s%s", r.code, tc.code, r.stdout, r.stderr)
			}
			var doc struct {
				Ended string `json:"ended"`
				Data  struct {
					Findings   []string `json:"findings"`
					Exemptions struct {
						Rows     float64  `json:"rows"`
						Findings []string `json:"findings"`
					} `json:"exemptions"`
				} `json:"data"`
			}
			if err := json.Unmarshal([]byte(r.stdout), &doc); err != nil {
				t.Fatalf("stdout is not one JSON document: %v\n%s", err, r.stdout)
			}
			if doc.Ended != tc.ended || doc.Data.Exemptions.Rows != tc.rows || len(doc.Data.Exemptions.Findings) != tc.findings || len(doc.Data.Findings) != tc.findings {
				t.Errorf("ended %q with %d rows, %d exemption findings and %d in all; want %q, %v and %d\n%s", doc.Ended, int(doc.Data.Exemptions.Rows), len(doc.Data.Exemptions.Findings), len(doc.Data.Findings), tc.ended, tc.rows, tc.findings, r.stdout)
			}
			uitest.Golden(t, "json-check-exemptions-"+tc.name, scrub(r.stdout, ""))
		})
	}
}

// TestAProjectWithNoExemptionsFileHasNoFindingAndNoError: the file is a
// project's choice, so a project without it is checked as before, and the
// answer holds no exemptions object.
func TestAProjectWithNoExemptionsFileHasNoFindingAndNoError(t *testing.T) {
	t.Parallel()
	r := run(t, fixture(t), nil, "check", "--json")
	if r.code != 0 || strings.Contains(r.stdout, "exemptions") || strings.Contains(r.stderr, "exemptions") {
		t.Errorf("exit %d, want 0 and no word of exemptions\n%s%s", r.code, r.stdout, r.stderr)
	}
}

// TestAnExemptionsExitLearnsNothingOfAFileOutsideTheProject: an exit names a
// file whose existence check reports, so an exit through a committed link
// to a file outside the project would tell the project that existence on
// the machine that runs check. check reads the project through
// its root, so it says the same whether the file outside is there or not.
func TestAnExemptionsExitLearnsNothingOfAFileOutsideTheProject(t *testing.T) {
	t.Parallel()
	repo := fixture(t)
	outside := filepath.Join(t.TempDir(), "outside.md")
	exemptionsAt(t, repo, "clean.yaml")
	data, err := os.ReadFile(filepath.Join(repo, "gate", "exemptions.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	planted := strings.Replace(string(data), `exit: "rulebook/parts.yaml"`, `exit: "linked.md"`, 1)
	if planted == string(data) {
		t.Fatal("the plant changed nothing")
	}
	if err := os.WriteFile(filepath.Join(repo, "gate", "exemptions.yaml"), []byte(planted), 0o644); err != nil {
		t.Fatal(err)
	}
	testenv.Link(t, outside, filepath.Join(repo, "linked.md"))
	if err := os.WriteFile(outside, []byte("a page\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	there := run(t, repo, nil, "check")
	if err := os.Remove(outside); err != nil {
		t.Fatal(err)
	}
	gone := run(t, repo, nil, "check")
	if there.code != 1 || gone.code != 1 || scrub(there.stdout+there.stderr, "") != scrub(gone.stdout+gone.stderr, "") || !strings.Contains(there.stdout, "its exit linked.md is a file that does") {
		t.Errorf("exit %d; check says one thing with the file outside there:\n%s%s\nand another without it:\n%s%s", there.code, there.stdout, there.stderr, gone.stdout, gone.stderr)
	}
}
