package root

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/config"
	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/exit"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// TestASettingsProblemEndsTheRunBeforeItLocksOrWrites: a run whose settings
// cannot be used ends on the problem's word before the command runs, even
// under --apply, so it takes no lock, opens no record and writes nothing.
// Risk: a run that writes with settings it was told are bad.
func TestASettingsProblemEndsTheRunBeforeItLocksOrWrites(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		problem error
		code    int
		says    []string
	}{
		{"a refusal", &config.Problem{Kind: config.Refused, What: settingsFile() + ":2: colour must be auto, always or never"},
			exit.Refused, []string{"refused", settingsFile() + ":2: colour must be auto, always or never"}},
		{"a newer file", &config.Problem{Kind: config.NotReady, What: settingsFile() + ":1: the settings file is version 2, and this gragas reads version 1", Next: "move " + settingsFile() + " away"},
			exit.NotReady, []string{"version 2", "move " + settingsFile() + " away"}},
		{"a file that cannot be read", &config.Problem{Kind: config.Failed, What: "the settings file " + settingsFile() + " could not be read: permission denied"},
			exit.Failed, []string{"failed", "could not be read: permission denied"}},
		{"an error that is not a problem", errors.New("the disk went away"),
			exit.Failed, []string{"failed", "the settings could not be read: the disk went away"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := world(t)
			p, _ := process(t, f, "gen", "--apply")
			p.Problem = tc.problem
			code, said := executeSaid(t.Context(), t, p)
			if code != tc.code {
				t.Errorf("exit %d, want %d; said:\n%s", code, tc.code, said)
			}
			for _, s := range tc.says {
				if !strings.Contains(said, s) {
					t.Errorf("said nothing of %q:\n%s", s, said)
				}
			}
			if len(f.Wrote) != 0 || f.Locked != 0 || len(f.Took) != 0 || len(f.Runs) != 0 {
				t.Errorf("wrote %v, locked %d, took %v, runs %d; want nothing", f.Wrote, f.Locked, f.Took, len(f.Runs))
			}
		})
	}
	t.Run("the clean control: no problem, and the run writes", func(t *testing.T) {
		t.Parallel()
		f := world(t)
		p, _ := process(t, f, "gen", "--apply")
		if code, said := executeSaid(t.Context(), t, p); code != exit.Done || len(f.Wrote) == 0 || f.Locked != 1 {
			t.Errorf("exit %d, wrote %v, locked %d; said:\n%s", code, f.Wrote, f.Locked, said)
		}
	})
}

// TestASettingsProblemIsTheAnswerUnderJSON: under --json the problem is the
// one document, refused, with the line it was refused at.
func TestASettingsProblemIsTheAnswerUnderJSON(t *testing.T) {
	t.Parallel()
	p, out := process(t, world(t), "gen", "--json")
	p.Problem = &config.Problem{Kind: config.Refused, What: settingsFile() + `:3: unknown key "colr": the keys are version, language`}
	if code, said := executeSaid(t.Context(), t, p); code != exit.Refused {
		t.Fatalf("exit %d, said:\n%s", code, said)
	}
	var r coreapi.Result
	if err := json.Unmarshal([]byte(out.String()), &r); err != nil {
		t.Fatalf("not one JSON document: %v\n%s", err, out)
	}
	if r.Ended != "refused" || r.Error == nil || r.Error.Kind != "refused" || !strings.Contains(r.Msg, settingsFile()+`:3: unknown key "colr"`) {
		t.Errorf("answer = %+v", r)
	}
}

// TestHelpPrintsWhenTheSettingsAreBad: --help reads no setting, so it is
// done and says nothing of them (docs/settings.md, known limits). The help
// itself goes to the process's stdout, which the test does not capture.
func TestHelpPrintsWhenTheSettingsAreBad(t *testing.T) {
	t.Parallel()
	p, out := process(t, world(t), "gen", "--help")
	p.Problem = &config.Problem{Kind: config.Refused, What: settingsFile() + ":2: colour must be auto, always or never"}
	code, said := executeSaid(t.Context(), t, p)
	if code != exit.Done || strings.Contains(out.String()+said, "colour must be") {
		t.Errorf("exit %d, said:\n%s%s", code, out, said)
	}
}

// unheld is every file the run in f wrote, locked or logged to that no row
// of table holds, and how many it checked.
func unheld(table []config.Kept, c config.Config, f *porttest.Fake) (missing []string, checked int) {
	for _, p := range f.Wrote {
		checked++
		if _, ok := config.Holds(table, config.InRepository, p); !ok {
			missing = append(missing, p)
		}
	}
	abs := append([]string{}, f.Took...)
	for _, r := range f.Runs {
		abs = append(abs, r.Path)
	}
	for _, p := range abs {
		checked++
		place, rel, ok := c.Where(p)
		if !ok {
			missing = append(missing, p)
			continue
		}
		if _, ok := config.Holds(table, place, rel); !ok {
			missing = append(missing, p)
		}
	}
	return missing, checked
}

// TestEveryFileARunKeepsIsInTheTable: what gen --apply writes, the lock it
// takes and the run log it adds are each a row of config.Persisted, and a
// table without the run log's row misses it. Risk: a file gragas keeps that
// docs/settings.md does not say it keeps.
func TestEveryFileARunKeepsIsInTheTable(t *testing.T) {
	t.Parallel()
	f := world(t)
	p, _ := process(t, f, "gen", "--apply")
	if code, said := executeSaid(t.Context(), t, p); code != exit.Done {
		t.Fatalf("exit %d, said:\n%s", code, said)
	}
	if len(f.Wrote) == 0 || len(f.Took) == 0 || len(f.Runs) == 0 {
		t.Fatalf("wrote %v, took %v, runs %d: the walk below checks nothing", f.Wrote, f.Took, len(f.Runs))
	}
	if missing, checked := unheld(config.Persisted(), p.Config, f); len(missing) != 0 || checked < 3 {
		t.Errorf("not in the table: %v (checked %d)", missing, checked)
	}
	var short []config.Kept
	for _, k := range config.Persisted() {
		if !slices.Contains(k.Names, "runs.jsonl") {
			short = append(short, k)
		}
	}
	if missing, _ := unheld(short, p.Config, f); len(missing) != 1 || !strings.HasSuffix(missing[0], "runs.jsonl") {
		t.Errorf("a table without the run log's row missed %v, want the run log alone", missing)
	}
}

// TestTheBaseRefIsNamedAsTheSettingsNameIt: check's refusal names the key
// and the variable config reads, which core cannot import.
func TestTheBaseRefIsNamedAsTheSettingsNameIt(t *testing.T) {
	t.Parallel()
	if coreapi.BaseRefVar != config.BaseRefVar {
		t.Errorf("core names %s, config reads %s", coreapi.BaseRefVar, config.BaseRefVar)
	}
	if !slices.ContainsFunc(config.Keys(), func(k config.Key) bool { return k.Name == coreapi.BaseRefKey && k.Env == config.BaseRefVar }) {
		t.Errorf("no settings key %s read from %s", coreapi.BaseRefKey, config.BaseRefVar)
	}
}
