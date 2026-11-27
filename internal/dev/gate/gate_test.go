package gate_test

import (
	"io/fs"
	"maps"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/dev/gate"
)

// files is a small repository: every registry file with one valid row.
func files() map[string]string {
	return map[string]string{
		gate.Exemptions: `exemptions:
  - {id: exit, rule: cli.exit.1, path: ".", reason: "the spec needs exit 6", exit: "x/rulebook#34", owner: gate, expires: "2027-03-31"}
`,
		gate.Standins:               "alex  # a person\n",
		gate.ClosureList:            "# modules\nexample.org/one\nexample.org/two\n",
		"gate/shared.yaml":          "shared:\n  - capability: lock\n    owner: internal/x\n  - capability: digest\n    owner: internal/digest\n    allow:\n      - {package: internal/journeys, why: \"sums alone\"}\n",
		"gate/schema/x.json":        "{}\n",
		gate.Subsets:                `{"subsets": [{"name": "a", "decision": "A61", "allowed": [{"package": "internal/x", "decision": "A61"}]}]}`,
		gate.ReasonsDir + "/A61.md": "# A61\n",
		gate.Crash:                  `{"crash": [{"command": "init", "seed": "empty", "args": ["init", "--apply"], "points": 4}]}`,
	}
}

// inputs reads from files with a validator that finds nothing.
func inputs(files map[string]string) gate.Inputs {
	return gate.Inputs{
		Read: func(p string) ([]byte, error) {
			if s, ok := files[p]; ok {
				return []byte(s), nil
			}
			return nil, &os.PathError{Op: "open", Path: p, Err: fs.ErrNotExist}
		},
		Validate: func(string, any) []string { return nil },
		Closure:  []string{"example.org/one", "example.org/two"},
		Names: func(dir string) ([]string, error) {
			var out []string
			for p := range files {
				if name, ok := strings.CutPrefix(p, dir+"/"); ok && !strings.Contains(name, "/") {
					out = append(out, name)
				}
			}
			return out, nil
		},
	}
}

// edit is a copy of m, changed by f.
func edit(m map[string]string, f func(map[string]string)) map[string]string {
	out := maps.Clone(m)
	f(out)
	return out
}

func fires(t *testing.T, found []string, want string) {
	t.Helper()
	if !slices.ContainsFunc(found, func(f string) bool { return strings.Contains(f, want) }) {
		t.Errorf("no finding says %q; found %q", want, found)
	}
}

// TestACleanRegistryHolds is the control every plant below is read against.
func TestACleanRegistryHolds(t *testing.T) {
	if found := gate.Check(inputs(files())).Findings; len(found) != 0 {
		t.Fatalf("the clean control fails: %q", found)
	}
}

// TestAMissingOrEmptyRegistryFileFailsTheGate: the gate fails closed on each
// file it reads, whether it is gone, blank or only comments (gates.plants.5).
func TestAMissingOrEmptyRegistryFileFailsTheGate(t *testing.T) {
	registry := []string{gate.Exemptions, gate.Crash, gate.Subsets, gate.Standins, gate.ClosureList}
	for _, f := range registry {
		t.Run(f+" missing", func(t *testing.T) {
			fires(t, gate.Check(inputs(edit(files(), func(m map[string]string) { delete(m, f) }))).Findings, f+": is missing")
		})
		t.Run(f+" blank", func(t *testing.T) {
			fires(t, gate.Check(inputs(edit(files(), func(m map[string]string) { m[f] = "\n  \n" }))).Findings, f+": holds nothing")
		})
		t.Run(f+" with no rows", func(t *testing.T) {
			empty := "# only a comment\n"
			if stem, ok := strings.CutSuffix(strings.TrimPrefix(f, "gate/"), ".yaml"); ok {
				empty = stem + ": []\n"
			}
			found := gate.Check(inputs(edit(files(), func(m map[string]string) { m[f] = empty }))).Findings
			fires(t, found, f+": ")
		})
	}
	t.Run("no schema check given", func(t *testing.T) {
		in := inputs(files())
		in.Validate = nil
		fires(t, gate.Check(in).Findings, "no schema check was given")
	})
}

// TestTheSchemasProblemsAreTheFindings: what the schema says is wrong with a
// file is reported under its name.
func TestTheSchemasProblemsAreTheFindings(t *testing.T) {
	in := inputs(files())
	in.Validate = func(schema string, _ any) []string {
		if schema == "exemptions" {
			return []string{"line 3: count is missing"}
		}
		return nil
	}
	fires(t, gate.Check(in).Findings, gate.Exemptions+": line 3: count is missing")
}

// TestTheClosureListOnlyFalls: a module linked and not listed fails, a module
// listed and not linked fails, and a closure that was not read fails.
func TestTheClosureListOnlyFalls(t *testing.T) {
	in := inputs(files())
	in.Closure = []string{"example.org/one", "example.org/two", "example.org/three"}
	fires(t, gate.Check(in).Findings, "links example.org/three, which the declared closure does not list")
	in.Closure = []string{"example.org/one"}
	fires(t, gate.Check(in).Findings, "example.org/two is listed and no longer linked")
	in.Closure = nil
	fires(t, gate.Check(in).Findings, "were not read")
}

func TestTheTwoWaysAnInputFailsReadDifferently(t *testing.T) {
	if gate.ErrMissing.Error() == gate.ErrEmpty.Error() {
		t.Error("the two ways an input fails read the same")
	}
}

// TestASubsetStandsOnADecisionRecord: a subset, and each package let past
// its check, names a decision record that is there, so a package joins a
// subset only with one (arch.layers.13); a subset listed twice
// fails too.
func TestASubsetStandsOnADecisionRecord(t *testing.T) {
	cases := []struct {
		claim, json, want string
	}{
		{"a subset whose decision is not there", `{"subsets": [{"name": "a", "decision": "A999"}]}`, "a names the decision A999, and docs/reasons holds no file named A999.md"},
		{"an allowed package whose decision is not there", `{"subsets": [{"name": "a", "decision": "A61", "allowed": [{"package": "internal/x", "decision": "B2"}]}]}`, "a allows internal/x names the decision B2"},
		{"a subset listed twice", `{"subsets": [{"name": "a", "decision": "A61"}, {"name": "a", "decision": "A61"}]}`, "the subset a is listed twice"},
	}
	for _, c := range cases {
		t.Run(c.claim, func(t *testing.T) {
			fires(t, gate.Check(inputs(edit(files(), func(m map[string]string) { m[gate.Subsets] = c.json }))).Findings, c.want)
		})
	}
}

// TestTheSubsetListOnlyShrinksAgainstTheBase: a row that lets more in than
// the base's stands on a decision record the base does not hold, and a row
// the base has cannot go; a narrower row, a new row and no base pass.
func TestTheSubsetListOnlyShrinksAgainstTheBase(t *testing.T) {
	const base = `{"subsets": [{"name": "a", "decision": "A61", "forbidden": ["x", "y"], "no_embeds": true, "allowed": [{"package": "p", "decision": "A61"}]}]}`
	cases := []struct {
		claim, head, want string
	}{
		{"a forbidden entry dropped", `{"subsets": [{"name": "a", "decision": "A61", "forbidden": ["x"], "no_embeds": true, "allowed": [{"package": "p", "decision": "A61"}]}]}`, "the subset a no longer forbids y, on the decision A61, which the base already holds"},
		{"no_embeds turned off", `{"subsets": [{"name": "a", "decision": "A61", "forbidden": ["x", "y"], "no_embeds": false, "allowed": [{"package": "p", "decision": "A61"}]}]}`, "the subset a no longer holds no_embeds"},
		{"a package allowed", `{"subsets": [{"name": "a", "decision": "A61", "forbidden": ["x", "y"], "no_embeds": true, "allowed": [{"package": "p", "decision": "A61"}, {"package": "q", "decision": "A61"}]}]}`, "the subset a allows q, which the base does not, on the decision A61"},
		{"a row the base has gone", `{"subsets": [{"name": "b", "decision": "A61", "forbidden": ["x"]}]}`, "the subset a, which the base holds, is gone"},
	}
	baseOf := func(decisions ...string) func(string) ([]byte, error) {
		return func(p string) ([]byte, error) {
			if p == gate.Subsets {
				return []byte(base), nil
			}
			if slices.Contains(decisions, p) {
				return []byte("# a decision\n"), nil
			}
			return nil, nil
		}
	}
	for _, c := range cases {
		t.Run(c.claim, func(t *testing.T) {
			in := inputs(edit(files(), func(m map[string]string) { m[gate.Subsets] = c.head }))
			in.Base = baseOf(gate.ReasonsDir + "/A61.md")
			fires(t, gate.Check(in).Findings, c.want)
		})
	}
	t.Run("a widening on a decision new against the base passes", func(t *testing.T) {
		in := inputs(edit(files(), func(m map[string]string) {
			m[gate.Subsets] = cases[0].head
			m[gate.ReasonsDir+"/A99.md"] = "# A99\n"
		}))
		in.Base = baseOf(gate.ReasonsDir + "/A61.md")
		in.Read = func(p string) ([]byte, error) {
			if p == gate.Subsets {
				return []byte(strings.Replace(cases[0].head, `"name": "a", "decision": "A61"`, `"name": "a", "decision": "A99"`, 1)), nil
			}
			return inputs(edit(files(), func(m map[string]string) { m[gate.ReasonsDir+"/A99.md"] = "# A99\n" })).Read(p)
		}
		if found := gate.Check(in).Findings; len(found) != 0 {
			t.Errorf("a widening on a new decision failed: %q", found)
		}
	})
	t.Run("a narrower row and a new row pass", func(t *testing.T) {
		in := inputs(edit(files(), func(m map[string]string) {
			m[gate.Subsets] = `{"subsets": [{"name": "a", "decision": "A61", "forbidden": ["x", "y", "z"], "no_embeds": true}, {"name": "c", "decision": "A61", "forbidden": ["x"]}]}`
		}))
		in.Base = baseOf(gate.ReasonsDir + "/A61.md")
		if found := gate.Check(in).Findings; len(found) != 0 {
			t.Errorf("a narrower list failed: %q", found)
		}
	})
	t.Run("a base that cannot be read fails", func(t *testing.T) {
		in := inputs(files())
		in.Base = func(string) ([]byte, error) { return nil, fs.ErrPermission }
		fires(t, gate.Check(in).Findings, "the base's list could not be read")
	})
}

// TestADroppedMarkerWidensTheSubset: a TypeScript row that no longer
// searches for a marker the base's row does lets more in.
func TestADroppedMarkerWidensTheSubset(t *testing.T) {
	in := inputs(edit(files(), func(m map[string]string) {
		m[gate.Subsets] = `{"subsets": [{"name": "a", "decision": "A61", "forbidden": ["x"]}]}`
	}))
	in.Base = func(p string) ([]byte, error) {
		switch p {
		case gate.Subsets:
			return []byte(`{"subsets": [{"name": "a", "decision": "A61", "forbidden": ["x"], "markers": ["here/view"]}]}`), nil
		case gate.ReasonsDir + "/A61.md":
			return []byte("# A61\n"), nil
		}
		return nil, nil
	}
	fires(t, gate.Check(in).Findings, "the subset a no longer searches for the marker here/view")
}

// TestARowThatBuildsAnotherThingWidensTheSubset: the same forbidden list over
// another build, entry or package holds that build to nothing it was held
// to, so each change of what a row builds is a widening, on a decision the
// base does not hold.
func TestARowThatBuildsAnotherThingWidensTheSubset(t *testing.T) {
	const goRow = `{"name": "a", "build": "go", "entry": "./cmd/one", "decision": "A61", "forbidden": ["x"], "no_embeds": true}`
	const tsRow = `{"name": "b", "build": "ts", "package": "element", "decision": "A61", "forbidden": ["x"]}`
	base := `{"subsets": [` + goRow + `, ` + tsRow + `]}`
	cases := []struct {
		claim, from, to, want string
	}{
		{"a Go row's entry", `"entry": "./cmd/one"`, `"entry": "./cmd/two"`, `the subset a builds another entry, "./cmd/two" where the base has "./cmd/one"`},
		{"a TypeScript row's package", `"package": "element"`, `"package": "contract"`, `the subset b builds another package, "contract" where the base has "element"`},
		{"a row's build", `"build": "ts", "package": "element"`, `"build": "go", "package": "element"`, `the subset b builds another build, "go" where the base has "ts"`},
	}
	for _, c := range cases {
		t.Run(c.claim, func(t *testing.T) {
			head := strings.Replace(base, c.from, c.to, 1)
			if head == base {
				t.Fatalf("the plant changed nothing")
			}
			in := inputs(edit(files(), func(m map[string]string) { m[gate.Subsets] = head }))
			in.Base = func(p string) ([]byte, error) {
				switch p {
				case gate.Subsets:
					return []byte(base), nil
				case gate.ReasonsDir + "/A61.md":
					return []byte("# A61\n"), nil
				}
				return nil, nil
			}
			fires(t, gate.Check(in).Findings, c.want+", on the decision A61, which the base already holds")
		})
	}
	t.Run("the same rows pass", func(t *testing.T) {
		in := inputs(edit(files(), func(m map[string]string) { m[gate.Subsets] = base }))
		in.Base = func(p string) ([]byte, error) {
			if p == gate.Subsets {
				return []byte(base), nil
			}
			if p == gate.Shared {
				return nil, nil
			}
			return []byte("# a decision\n"), nil
		}
		if found := gate.Check(in).Findings; len(found) != 0 {
			t.Errorf("the base's own rows failed: %q", found)
		}
	})
}

// TestADecisionIsFoundByItsExactName: on a file system that folds case,
// docs/reasons/a53.md opens A53.md, and git, which the base is read from,
// lists A53.md alone; so the record is found by its exact name in the
// listing, and a53 is held to no file. The plant
// reads as such a file system does: any case of a name opens the file.
func TestADecisionIsFoundByItsExactName(t *testing.T) {
	m := edit(files(), func(m map[string]string) {
		m[gate.Subsets] = `{"subsets": [{"name": "element", "decision": "a61"}]}`
	})
	in := inputs(m)
	read := in.Read
	in.Read = func(p string) ([]byte, error) { return read(strings.Replace(p, "/a61.md", "/A61.md", 1)) }
	fires(t, gate.Check(in).Findings, "element names the decision a61, and docs/reasons holds no file named a61.md")
	t.Run("the control, its exact name", func(t *testing.T) {
		in := inputs(edit(m, func(m map[string]string) { m[gate.Subsets] = `{"subsets": [{"name": "element", "decision": "A61"}]}` }))
		if found := gate.Check(in).Findings; len(found) != 0 {
			t.Errorf("the record by its exact name failed: %q", found)
		}
	})
}

// TestABaseWithNoListIsAReportLine: the head's list held to a base that has
// none holds nothing, and the gate says so on a report line rather than in
// silence.
func TestABaseWithNoListIsAReportLine(t *testing.T) {
	in := inputs(files())
	in.Base = func(string) ([]byte, error) { return nil, nil }
	rep := gate.Check(in)
	if len(rep.Findings) != 0 || !slices.Contains(rep.Lengths, "subsets: the base holds no list, so none was held to it") {
		t.Errorf("a base with no list gave findings %q and report lines %q", rep.Findings, rep.Lengths)
	}
}

// TestTheSharedRowsOnlyShrinkAgainstTheBase: what a row of gate/shared.yaml
// lets in that the base's did not stands on a decision record the base does
// not hold: a package added to allow, a tests reprieve added or ended on
// another wait, a row turned back to later. The same file, a narrower one and
// a widening on a new decision pass.
func TestTheSharedRowsOnlyShrinkAgainstTheBase(t *testing.T) {
	const base = `shared:
  - capability: lock
    owner: internal/host/fs
    status: shared
  - capability: digest
    owner: internal/digest
    primitives:
      - call: crypto/sha256.New
      - call: crypto/sha256.Sum256
    allow:
      - {package: internal/journeys, why: "sums alone"}
    status: local
  - capability: atomic-write
    owner: internal/host/fs
    tests:
      spared: "test files make the raw calls"
      until: "the raw calls move onto the helper"
    status: shared
  - capability: splice
    owner: internal/rendered
    status: later
    until: "the old copies are gone"
`
	with := func(old, now string) string {
		if !strings.Contains(base, old) {
			t.Fatalf("the base holds no %q", old)
		}
		return strings.Replace(base, old, now, 1)
	}
	cases := []struct {
		claim, head, want string
	}{
		{"an allow entry added", with("sums alone\"}\n", "sums alone\"}\n      - {package: internal/x, why: \"sums as well\"}\n"), "the row digest allows internal/x beside its owner, which the base's row does not, and names no decision"},
		{"a tests reprieve added to the lock row", with("    status: shared\n  - capability: digest", "    tests:\n      spared: \"test files are spared\"\n      until: \"the lock moves\"\n    status: shared\n  - capability: digest"), "the row lock spares its test files, which the base's row does not, and names no decision"},
		{"a reprieve that ends on another wait", with("the raw calls move onto the helper", "a helper is written one day"), "the row atomic-write spares its test files until a helper is written one day, where the base's reprieve ends on the raw calls move onto the helper"},
		{"a reprieve in words that goes back to a number", with("the raw calls move onto the helper", "#9999"), "the row atomic-write spares its test files until #9999, where the base's reprieve ends on the raw calls move onto the helper, and names no decision"},
		{"a row turned back to later", with("    status: local", "    status: later"), "the row digest is later again, where the base's is local"},
		{"a later row that waits on another wait", with("the old copies are gone", "the copies are never gone"), "the row splice is later until the copies are never gone, where the base's waits on the old copies are gone: its ban is put off, and names no decision"},
		{"a later row in words that goes back to a number", with("the old copies are gone", "#9999"), "the row splice is later until #9999, where the base's waits on the old copies are gone: its ban is put off, and names no decision"},
		{"a later row that waits on another wait, on a decision the base holds", with("    until: \"the old copies are gone\"\n", "    until: \"the copies are never gone\"\n    decision: A61\n"), "its ban is put off, on the decision A61, which the base already holds"},
		{"a row removed", with("  - capability: digest\n", "  - capability: digestx\n"), "the row digest, which the base holds, is gone or renamed: its bans are dropped, and names no decision"},
		{"a row renamed on a record the base holds", with("  - capability: digest\n", "  - capability: digestx\n") + "removed:\n  - {capability: digest, decision: A61, why: \"renamed to digestx\"}\n", "is gone or renamed: its bans are dropped, on the decision A61, which the base already holds"},
		{"a primitive dropped", with("      - call: crypto/sha256.Sum256\n", ""), "the row digest no longer bans the primitive call crypto/sha256.Sum256, and names no decision"},
		{"an owner moved", with("    owner: internal/digest\n", "    owner: internal/other\n"), "the row digest moves to the owner internal/other, where the base's is internal/digest: the ban's exclusion follows it, and names no decision"},
		{"a widening on a decision no record has", with("sums alone\"}\n", "sums alone\"}\n      - {package: internal/x, why: \"sums as well\", decision: A7}\n"), "docs/reasons holds no file named A7.md"},
		{"a widening on a decision the base holds", with("sums alone\"}\n", "sums alone\"}\n      - {package: internal/x, why: \"sums as well\", decision: A61}\n"), "on the decision A61, which the base already holds"},
	}
	baseOf := func(p string) ([]byte, error) {
		switch p {
		case gate.Shared:
			return []byte(base), nil
		case gate.ReasonsDir + "/A61.md":
			return []byte("# A61\n"), nil
		}
		return nil, nil
	}
	run := func(head string, more map[string]string) []string {
		in := inputs(edit(files(), func(m map[string]string) {
			m[gate.Shared] = head
			maps.Copy(m, more)
		}))
		in.Base = baseOf
		return gate.Check(in).Findings
	}
	for _, c := range cases {
		t.Run(c.claim, func(t *testing.T) {
			fires(t, run(c.head, nil), c.want)
		})
	}
	t.Run("the base's own file passes", func(t *testing.T) {
		if found := run(base, nil); len(found) != 0 {
			t.Errorf("the same file failed: %q", found)
		}
	})
	t.Run("a row added and a primitive added pass", func(t *testing.T) {
		head := with("      - call: crypto/sha256.Sum256\n", "      - call: crypto/sha256.Sum256\n      - call: crypto/sha256.Sum224\n") + "  - capability: sort\n    owner: internal/sorts\n    status: local\n"
		if found := run(head, nil); len(found) != 0 {
			t.Errorf("a row and a primitive added failed: %q", found)
		}
	})
	// A base from before the key waits on nothing, so the head naming an issue
	// is no widening; a later row turned local is a narrowing.
	noUntil := with("    until: \"the old copies are gone\"\n", "")
	// A base whose untils name issues by their numbers, as every base did
	// before the words: the head that says the same waits in words is the
	// same file, so it passes.
	numbered := strings.Replace(with("the raw calls move onto the helper", "#1751"), "the old copies are gone", "go-files#374", 1)
	for _, c := range []struct{ claim, base, head string }{
		{"an until named where the base's row has none passes", noUntil, base},
		{"a later row turned local passes", base, with("    status: later\n    until: \"the old copies are gone\"\n", "    status: local\n")},
		{"an until that named an issue, put in words, passes", numbered, base},
	} {
		t.Run(c.claim, func(t *testing.T) {
			in := inputs(edit(files(), func(m map[string]string) { m[gate.Shared] = c.head }))
			in.Base = func(p string) ([]byte, error) {
				if p == gate.Shared {
					return []byte(c.base), nil
				}
				return baseOf(p)
			}
			if found := gate.Check(in).Findings; len(found) != 0 {
				t.Errorf("a narrowing failed: %q", found)
			}
		})
	}
	// Against that numbered base, the rename excuses the until alone: another
	// widening of the same row, and a number moved to another number, still
	// stand on a decision.
	for _, c := range []struct{ claim, head, want string }{
		{"a rename with an allow entry added", strings.Replace(base, "sums alone\"}\n", "sums alone\"}\n      - {package: internal/x, why: \"sums as well\"}\n", 1), "the row digest allows internal/x beside its owner, which the base's row does not, and names no decision"},
		{"a rename with a primitive dropped", strings.Replace(base, "      - call: crypto/sha256.Sum256\n", "", 1), "the row digest no longer bans the primitive call crypto/sha256.Sum256, and names no decision"},
		{"a number moved to another number", strings.Replace(numbered, "go-files#374", "go-files#375", 1), "the row splice is later until go-files#375, where the base's waits on go-files#374: its ban is put off, and names no decision"},
		{"a reprieve's number moved to another number", strings.Replace(numbered, "#1751", "#1752", 1), "the row atomic-write spares its test files until #1752, where the base's reprieve ends on #1751, and names no decision"},
	} {
		t.Run(c.claim, func(t *testing.T) {
			in := inputs(edit(files(), func(m map[string]string) { m[gate.Shared] = c.head }))
			in.Base = func(p string) ([]byte, error) {
				if p == gate.Shared {
					return []byte(numbered), nil
				}
				return baseOf(p)
			}
			fires(t, gate.Check(in).Findings, c.want)
		})
	}
	t.Run("a row removed on a new decision passes", func(t *testing.T) {
		head := with("  - capability: digest\n", "  - capability: digestx\n") + "removed:\n  - {capability: digest, decision: A99, why: \"renamed to digestx\"}\n"
		if found := run(head, map[string]string{gate.ReasonsDir + "/A99.md": "# A99\n"}); len(found) != 0 {
			t.Errorf("a rename on a new decision failed: %q", found)
		}
	})
	// The decision keys are read from the head against a base that has none,
	// so each case goes red if the gate stops reading its key;
	// cmd/gragas-dev's TestTheDecisionKeysAreReadByLayersCheckToo puts the
	// same keys through layers --check.
	newDecision := map[string]string{gate.ReasonsDir + "/A99.md": "# A99\n"}
	t.Run("a row's decision lets a primitive be dropped", func(t *testing.T) {
		head := strings.Replace(with("      - call: crypto/sha256.Sum256\n", ""), "    status: local", "    status: local\n    decision: A99", 1)
		if found := run(head, newDecision); len(found) != 0 {
			t.Errorf("a primitive dropped on the row's decision failed: %q", found)
		}
	})
	t.Run("a tests reprieve's decision lets it end on another wait", func(t *testing.T) {
		head := with("      until: \"the raw calls move onto the helper\"\n", "      until: \"a helper is written one day\"\n      decision: A99\n")
		if found := run(head, newDecision); len(found) != 0 {
			t.Errorf("a reprieve moved on its own decision failed: %q", found)
		}
	})
	t.Run("a narrower file passes", func(t *testing.T) {
		head := with("    allow:\n      - {package: internal/journeys, why: \"sums alone\"}\n", "")
		if found := run(head, nil); len(found) != 0 {
			t.Errorf("a narrower file failed: %q", found)
		}
	})
	t.Run("a widening on a new decision passes", func(t *testing.T) {
		head := with("sums alone\"}\n", "sums alone\"}\n      - {package: internal/x, why: \"sums as well\", decision: A99}\n")
		if found := run(head, map[string]string{gate.ReasonsDir + "/A99.md": "# A99\n"}); len(found) != 0 {
			t.Errorf("a widening on a new decision failed: %q", found)
		}
	})
	t.Run("a base that cannot be read fails", func(t *testing.T) {
		in := inputs(files())
		in.Base = func(string) ([]byte, error) { return nil, fs.ErrPermission }
		fires(t, gate.Check(in).Findings, "gate/shared.yaml: the base's file could not be read")
	})
}
