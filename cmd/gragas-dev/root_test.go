package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestARegistryRowLearnsNothingOfAFileOutsideTheRepository: a baseline row
// prints the line count of its path and an exemption's exit is checked for
// existence, so a row naming a file outside the repository, by a path that
// climbs out, starts at a root or goes through a committed link, would tell
// a head that count or that existence on the machine that runs the gate
// (gragas#498). Every read goes through the repository's root, so the gate
// says the same whether the file outside is there or not.
func TestARegistryRowLearnsNothingOfAFileOutsideTheRepository(t *testing.T) {
	t.Parallel()
	type row struct{ file, from, to string }
	rows := map[string]row{
		"an exemption row": {filepath.Join("gate", "exemptions.yaml"), "exit: rule-source/amendments#34", "exit: "},
	}
	paths := map[string]func(t *testing.T, dir, outside string) string{
		"a path that climbs out": func(_ *testing.T, _, _ string) string { return "../outside.txt" },
		"an absolute path":       func(_ *testing.T, _, outside string) string { return filepath.ToSlash(outside) },
		"a link to the file": func(t *testing.T, dir, outside string) string {
			t.Helper()
			relink(t, filepath.Join(dir, "linked.txt"), outside)
			return "linked.txt"
		},
		"a link to its folder": func(t *testing.T, dir, outside string) string {
			t.Helper()
			relink(t, filepath.Join(dir, "out"), filepath.Dir(outside))
			return "out/outside.txt"
		},
	}
	for rname, r := range rows {
		for pname, plant := range paths {
			t.Run(rname+", "+pname, func(t *testing.T) {
				t.Parallel() // its own copy of the gate files and its own outside file
				dir := copyGate(t)
				outside := filepath.Join(filepath.Dir(dir), "outside.txt")
				p := plant(t, dir, outside)
				file := filepath.Join(dir, r.file)
				data, err := os.ReadFile(file)
				if err != nil {
					t.Fatal(err)
				}
				edited := strings.Replace(string(data), r.from, r.to+p, 1)
				if edited == string(data) {
					t.Fatalf("%s holds no %q to plant", r.file, r.from)
				}
				if err := os.WriteFile(file, []byte(edited), 0o644); err != nil {
					t.Fatal(err)
				}
				m := fakeMachine(t, dir)
				if err := os.WriteFile(outside, []byte("one\ntwo\nthree\nfour\nfive\nsix\nseven\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				code, there, _ := doGate(m, dir)
				if err := os.Remove(outside); err != nil {
					t.Fatal(err)
				}
				_, gone, _ := doGate(m, dir)
				if code != exitFailed || there != gone {
					t.Errorf("exit %d; the gate says one thing with the file outside there:\n%s\nand another without it:\n%s", code, there, gone)
				}
			})
		}
	}
}
