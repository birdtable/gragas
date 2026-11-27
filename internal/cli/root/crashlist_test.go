package root

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// The crash list (store.recovery.2): gate/crash.json names every
// command of the tree that writes, each crash-injected by a workload, covered
// by a scripted crash test, or outside the journalled write path with the
// issue that moves it. Its journey is internal/journeys/crash_commands_test.go.

// crashRow is one row of gate/crash.json, as much as the coverage reads.
type crashRow struct {
	Command  string   `json:"command"`
	Args     []string `json:"args"`
	Scripted string   `json:"scripted"`
	Outside  string   `json:"outside"`
	Covers   []string `json:"covers"`
}

// crashList is gate/crash.json, read from the repository.
func crashList(t *testing.T) []crashRow {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "gate", "crash.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Crash []crashRow `json:"crash"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("gate/crash.json: %v", err)
	}
	return doc.Crash
}

// crashListProblems is what is wrong with the list, held to the families of
// the tree that write and to the crash journey's source: a writer with no
// row or with two, a row of no writer of the tree, a workload whose command
// line is not its family's, and a scripted row whose test the source does
// not define.
func crashListProblems(rows []crashRow, writers []string, journeys string) []string {
	var out []string
	seen := map[string]int{}
	for _, r := range rows {
		seen[r.Command]++
		kinds := 0
		for _, set := range []bool{r.Args != nil, r.Scripted != "", r.Outside != ""} {
			if set {
				kinds++
			}
		}
		switch {
		case kinds != 1:
			out = append(out, fmt.Sprintf("the row of %s is %d kinds, not one of a workload, a scripted test or outside", r.Command, kinds))
		case r.Outside != "" && !slices.Contains(outsideWriters(), r.Command):
			out = append(out, fmt.Sprintf("%s is put outside the write path, which only %v are: a command of the write path is killed", r.Command, outsideWriters()))
		case r.Scripted != "" && !slices.Contains(scriptedWriters(), r.Command):
			out = append(out, fmt.Sprintf("%s is covered by a scripted test, which only %v is: it is killed as itself", r.Command, scriptedWriters()))
		case !slices.Contains(writers, r.Command):
			out = append(out, fmt.Sprintf("the crash list names %s, which is no command of the tree that writes", r.Command))
		case r.Args != nil && r.Args[0] != r.Command:
			out = append(out, fmt.Sprintf("the workload of %s runs %q, another command", r.Command, strings.Join(r.Args, " ")))
		case r.Scripted != "" && !strings.Contains(journeys, "func "+r.Scripted+"(t *testing.T)"):
			out = append(out, fmt.Sprintf("%s is covered by %s, which the crash journey does not define", r.Command, r.Scripted))
		}
	}
	for _, w := range writers {
		switch seen[w] {
		case 0:
			out = append(out, fmt.Sprintf("%s writes and the crash list has no row for it: give it a workload, or an outside row with the issue that moves it onto the write path", w))
		case 1:
		default:
			out = append(out, fmt.Sprintf("the crash list has %d rows for %s", seen[w], w))
		}
	}
	return out
}

// annotatedWriters are the families of the tree with a command that carries
// the writes annotation, the code's own mark of a command that writes
// (cli.commands.1): the first word under the root of each such command.
func annotatedWriters(tree *cobra.Command) []string {
	var out []string
	var walk func(c *cobra.Command, family string)
	walk = func(c *cobra.Command, family string) {
		if _, ok := c.Annotations[writes]; ok && family != "" && !slices.Contains(out, family) {
			out = append(out, family)
		}
		for _, k := range c.Commands() {
			f := family
			if f == "" {
				f = k.Name()
			}
			walk(k, f)
		}
	}
	walk(tree, "")
	slices.Sort(out)
	return out
}

// registryWriters are the families of the tree whose registry row names
// what they write.
func registryWriters() []string {
	var out []string
	for _, c := range commands().InTree() {
		if len(c.Writes) > 0 {
			out = append(out, c.Name)
		}
	}
	slices.Sort(out)
	return out
}

// writersDisagree is where the registry's writes and the annotation part: a
// family the code marks as writing whose row names nothing it writes, or the
// other way round.
func writersDisagree(annotated, registry []string) []string {
	var out []string
	for _, w := range annotated {
		if !slices.Contains(registry, w) {
			out = append(out, w+" carries the writes annotation and its registry row names nothing it writes")
		}
	}
	for _, w := range registry {
		if !slices.Contains(annotated, w) {
			out = append(out, w+"'s registry row names what it writes and no command of it carries the writes annotation")
		}
	}
	return out
}

// TestEveryWritingCommandIsOnTheCrashList holds gate/crash.json to the
// registry both ways (store.recovery.2): every command of the
// tree that writes has one row of one kind, every row is such a command, and
// only the commands outside the write path today, and journal, are not
// killed as themselves. The plants are a writer the list leaves out, a row of
// a command that writes nothing, a workload put outside or made scripted, a
// row of no kind, a workload of another command and a scripted row whose test
// is gone. Risk: a writing command that enters the tree and is never killed
// mid-write.
func TestEveryWritingCommandIsOnTheCrashList(t *testing.T) {
	t.Parallel()
	rows, writers := crashList(t), annotatedWriters(tree(t))
	if bad := writersDisagree(writers, registryWriters()); len(bad) > 0 {
		t.Errorf("the writes annotation and the registry disagree:\n%s", strings.Join(bad, "\n"))
	}
	src, err := os.ReadFile(filepath.Join("..", "..", "journeys", "crash_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	if len(writers) < 5 {
		t.Fatalf("the registry names %d writing commands in the tree, %v: the coverage reads nothing", len(writers), writers)
	}
	if bad := crashListProblems(rows, writers, string(src)); len(bad) > 0 {
		t.Errorf("gate/crash.json does not hold:\n%s", strings.Join(bad, "\n"))
	}
	plants := []struct {
		claim string
		rows  []crashRow
		want  string
	}{
		{"a writer the list leaves out", rows[1:], rows[0].Command + " writes and the crash list has no row for it"},
		{"a row of a command that writes nothing", append(slices.Clone(rows), crashRow{Command: "version", Args: []string{"version"}}), "the crash list names version, which is no command of the tree that writes"},
		{"a workload of another command", append([]crashRow{{Command: rows[0].Command, Args: []string{"version", "--apply"}}}, rows[1:]...), "the workload of " + rows[0].Command + " runs \"version --apply\""},
		{"a scripted row whose test is gone", append(slices.Clone(rows), crashRow{Command: "journal", Scripted: "TestACrashThatIsGone"}), "TestACrashThatIsGone, which the crash journey does not define"},
		{"a workload put outside", append([]crashRow{{Command: "rule", Outside: "planted"}}, slices.DeleteFunc(slices.Clone(rows), func(r crashRow) bool { return r.Command == "rule" })...), "rule is put outside the write path"},
		{"a workload made scripted", append([]crashRow{{Command: "rule", Scripted: "TestACrashInsideTheRollBackIsRolledBackAgain"}}, slices.DeleteFunc(slices.Clone(rows), func(r crashRow) bool { return r.Command == "rule" })...), "rule is covered by a scripted test"},
		{"a row of no kind", append([]crashRow{{Command: "rule"}}, slices.DeleteFunc(slices.Clone(rows), func(r crashRow) bool { return r.Command == "rule" })...), "the row of rule is 0 kinds"},
	}
	for _, p := range plants {
		t.Run(p.claim, func(t *testing.T) {
			t.Parallel()
			got := strings.Join(crashListProblems(p.rows, writers, string(src)), "\n")
			if !strings.Contains(got, p.want) {
				t.Errorf("the plant gave %q, want it to say %q", got, p.want)
			}
		})
	}
}

// outsideWriters are the commands that write outside the journalled write
// path today: a row may put no other outside it, so a writer
// cannot leave the kills by an edit of the list alone. plugin is approve, which
// records an approval in the machine's trust record, outside every root and
// whole under its own lock.
func outsideWriters() []string { return []string{"cache", "contract", "gen", "plugin"} }

// scriptedWriters are the commands a scripted crash test covers: journal
// recover is the roll-back the scripted kill inside the roll-back exercises.
func scriptedWriters() []string { return []string{"journal"} }

// TestTheWritersComeFromTheAnnotation (on): a family
// writes when any command of it carries the writes annotation, whatever its
// registry row says; a planted tree names its marked families and no other,
// and a registry that names a writer the code does not mark, or leaves out
// one it does, is a finding each way beside the agreeing control.
func TestTheWritersComeFromTheAnnotation(t *testing.T) {
	t.Parallel()
	marked := func(name string) *cobra.Command {
		return &cobra.Command{Use: name, Annotations: map[string]string{writes: "with --apply"}}
	}
	root, a, b := &cobra.Command{Use: "gragas"}, &cobra.Command{Use: "a"}, &cobra.Command{Use: "b"}
	a.AddCommand(marked("x"), &cobra.Command{Use: "y"})
	b.AddCommand(&cobra.Command{Use: "z"})
	root.AddCommand(a, b, marked("c"))
	if got := annotatedWriters(root); !slices.Equal(got, []string{"a", "c"}) {
		t.Errorf("annotatedWriters = %v, want [a c]: a by its marked subcommand, c by itself, b not", got)
	}
	if bad := writersDisagree([]string{"a", "c"}, []string{"a", "c"}); len(bad) != 0 {
		t.Errorf("the agreeing control gave %v", bad)
	}
	bad := strings.Join(writersDisagree([]string{"a", "b"}, []string{"a", "c"}), "\n")
	for _, want := range []string{"b carries the writes annotation and its registry row names nothing it writes", "c's registry row names what it writes and no command of it carries the writes annotation"} {
		if !strings.Contains(bad, want) {
			t.Errorf("the planted disagreement gave %q, want %q", bad, want)
		}
	}
}

// annotatedCommands are the commands of the tree that carry the writes
// annotation, each by its path under the root ("rule new", "gen").
func annotatedCommands(tree *cobra.Command) []string {
	var out []string
	var walk func(c *cobra.Command, at []string)
	walk = func(c *cobra.Command, at []string) {
		if _, ok := c.Annotations[writes]; ok && len(at) > 0 {
			out = append(out, strings.Join(at, " "))
		}
		for _, k := range c.Commands() {
			walk(k, append(slices.Clone(at), k.Name()))
		}
	}
	walk(tree, nil)
	slices.Sort(out)
	return out
}

// coversProblems holds the list to each writing command: every command that carries the writes annotation is named in
// the covers of exactly one row, the row of its family; every name a row
// covers is such a command; and a workload runs one of the commands its row
// covers. So a writer that enters an existing family unnamed fails.
func coversProblems(rows []crashRow, commands []string) []string {
	var out []string
	named := map[string]int{}
	for _, r := range rows {
		ran := r.Args == nil
		for _, c := range r.Covers {
			named[c]++
			if !slices.Contains(commands, c) {
				out = append(out, fmt.Sprintf("the row of %s covers %q, which carries no writes annotation", r.Command, c))
			}
			if family, _, _ := strings.Cut(c, " "); family != r.Command {
				out = append(out, fmt.Sprintf("the row of %s covers %q, a command of another family", r.Command, c))
			}
			if f := strings.Fields(c); len(r.Args) >= len(f) && slices.Equal(r.Args[:len(f)], f) {
				ran = true
			}
		}
		if !ran {
			out = append(out, fmt.Sprintf("the workload of %s runs %q, which is none of the commands its row covers %v", r.Command, strings.Join(r.Args, " "), r.Covers))
		}
	}
	for _, c := range commands {
		switch named[c] {
		case 0:
			out = append(out, fmt.Sprintf("%s writes and no crash-list row names it: add it to its family's covers", c))
		case 1:
		default:
			out = append(out, fmt.Sprintf("%s is named by %d crash-list rows", c, named[c]))
		}
	}
	return out
}

// TestEveryWritingCommandIsNamedOnTheCrashList holds the tree's annotated
// commands to the covers of gate/crash.json,
// with plants beside the list as committed: a new writer in an existing
// family that no row names, a covered name that carries no annotation, a
// command named by two rows, and a workload that runs none of its covers.
func TestEveryWritingCommandIsNamedOnTheCrashList(t *testing.T) {
	t.Parallel()
	rows, commands := crashList(t), annotatedCommands(tree(t))
	if len(commands) < 20 {
		t.Fatalf("the tree has %d commands that carry the writes annotation, %v: the walk read nothing", len(commands), commands)
	}
	if bad := coversProblems(rows, commands); len(bad) > 0 {
		t.Errorf("gate/crash.json does not name every writing command:\n%s", strings.Join(bad, "\n"))
	}
	ruleRow := slices.IndexFunc(rows, func(r crashRow) bool { return r.Command == "rule" })
	if ruleRow < 0 {
		t.Fatal("the crash list has no rule row to plant on")
	}
	with := func(f func(r *crashRow)) []crashRow {
		out := slices.Clone(rows)
		r := out[ruleRow]
		r.Covers, r.Args = slices.Clone(r.Covers), slices.Clone(r.Args)
		f(&r)
		out[ruleRow] = r
		return out
	}
	for _, p := range []struct {
		claim    string
		rows     []crashRow
		commands []string
		want     string
	}{
		{"a new writer in an existing family", rows, append(slices.Clone(commands), "rule frobnicate"), "rule frobnicate writes and no crash-list row names it"},
		{"a covered name with no annotation", with(func(r *crashRow) { r.Covers = append(r.Covers, "rule nonsense") }), commands, `the row of rule covers "rule nonsense", which carries no writes annotation`},
		{"a command named twice", append(slices.Clone(rows), crashRow{Command: "rule", Covers: []string{"rule new"}, Outside: "planted"}), commands, "rule new is named by 2 crash-list rows"},
		{"a workload of none of its covers", with(func(r *crashRow) { r.Args[1] = "frobnicate" }), commands, "which is none of the commands its row covers"},
	} {
		t.Run(p.claim, func(t *testing.T) {
			t.Parallel()
			if got := strings.Join(coversProblems(p.rows, p.commands), "\n"); !strings.Contains(got, p.want) {
				t.Errorf("the plant gave %q, want it to say %q", got, p.want)
			}
		})
	}
}
