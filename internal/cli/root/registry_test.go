package root

import (
	"github.com/birdtable/gragas/internal/cli/kit"
	configtest "github.com/birdtable/gragas/internal/config/testing"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/birdtable/gragas/internal/model"
)

const repoRoot = "../../.."

// specRow is one row of the table of spec 15.1, read from the document.
type specRow struct {
	name    string
	then    []string
	context kit.Context
	plan    bool
	exits   []model.Word
}

// specTable reads the commands of spec 15.1 and the two counts its prose
// states under it, so a test holds the registry to the document and copies no
// number.
func specTable(t *testing.T) (rows []specRow, commands, plans int) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(repoRoot, "docs/spec.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	_, section, _ := strings.Cut(text, "### 15.1 The full table")
	section, _, _ = strings.Cut(section, "### 15.2")
	paren := regexp.MustCompile(`\([^)]*\)`)
	for line := range strings.SplitSeq(section, "\n") {
		if !strings.HasPrefix(line, "| ") || strings.HasPrefix(line, "| Domain") || strings.HasPrefix(line, "|---") {
			continue
		}
		cols := strings.Split(strings.Trim(line, "|"), " | ")
		if len(cols) != 5 {
			t.Fatalf("a row of 15.1 has %d columns: %s", len(cols), line)
		}
		cmd, ctx, ends, closing := strings.TrimSpace(cols[1]), strings.TrimSpace(cols[2]), strings.TrimSpace(cols[3]), strings.TrimSpace(cols[4])
		if cmd == "(bare)" {
			continue
		}
		var exits []model.Word
		for w := range strings.SplitSeq(paren.ReplaceAllString(closing, ""), ";") {
			exits = append(exits, model.Word(strings.TrimSpace(w)))
		}
		plan := strings.HasPrefix(ends, "plan") || strings.Contains(ends, "a plan")
		base := specRow{context: kit.Context(ctx[:1]), plan: plan, exits: exits}
		switch {
		case strings.Contains(cmd, "("):
			name, inner, _ := strings.Cut(cmd, " (")
			base.name = name
			base.then = splitWords(strings.TrimSuffix(inner, ")"))
			rows = append(rows, base)
		case strings.Contains(strings.Split(cmd, ", ")[0], " "):
			first, rest, _ := strings.Cut(cmd, ", ")
			name, verb, _ := strings.Cut(first, " ")
			base.name = name
			base.then = append([]string{verb}, splitWords(rest)...)
			rows = append(rows, base)
		default:
			for _, name := range splitWords(cmd) {
				r := base
				r.name = name
				rows = append(rows, r)
			}
		}
	}
	m := regexp.MustCompile(`(?m)^([A-Z][a-z]+(?:-[a-z]+)?) named commands and the bare menu`).FindStringSubmatch(text)
	p := regexp.MustCompile(`(?m)^[A-Za-z, -]*?\. ([A-Z][a-z]+(?:-[a-z]+)?) of them end in a plan`).FindStringSubmatch(text)
	if m == nil || p == nil {
		t.Fatal("the prose under 15.1 no longer states the two counts the test reads")
	}
	return rows, number(t, m[1]), number(t, p[1])
}

func splitWords(s string) []string {
	var out []string
	for w := range strings.SplitSeq(s, ",") {
		out = append(out, strings.TrimSpace(w))
	}
	return out
}

// number reads an English number up to ninety-nine.
func number(t *testing.T, w string) int {
	t.Helper()
	ones := []string{"zero", "one", "two", "three", "four", "five", "six", "seven", "eight", "nine", "ten", "eleven", "twelve",
		"thirteen", "fourteen", "fifteen", "sixteen", "seventeen", "eighteen", "nineteen"}
	tens := []string{"", "", "twenty", "thirty", "forty", "fifty", "sixty", "seventy", "eighty", "ninety"}
	tensPart, onesPart, _ := strings.Cut(strings.ToLower(w), "-")
	if i := slices.Index(ones, tensPart); i >= 0 && onesPart == "" {
		return i
	}
	i := slices.Index(tens, tensPart)
	if i < 2 {
		t.Fatalf("%q is not a number the test reads", w)
	}
	if onesPart == "" {
		return i * 10
	}
	j := slices.Index(ones, onesPart)
	if j < 1 {
		t.Fatalf("%q is not a number the test reads", w)
	}
	return i*10 + j
}

// TestTheRegistryHoldsTheCommandsOfSpec15_1: every row of the table is a row
// of the registry with the same words, context, plan ending and closing
// words, no row is extra but gen, and the two counts the prose states are the
// registry's own.
func TestTheRegistryHoldsTheCommandsOfSpec15_1(t *testing.T) {
	t.Parallel()
	spec, nCommands, nPlans := specTable(t)
	reg := commands()
	var inSpec []kit.Command
	for _, c := range reg.Rows() {
		if c.Spec {
			inSpec = append(inSpec, c)
		}
	}
	if len(inSpec) != nCommands || len(spec) != nCommands {
		t.Errorf("the registry has %d commands of the spec, the spec states %d and its table holds %d rows", len(inSpec), nCommands, len(spec))
	}
	plans := 0
	for _, want := range spec {
		got, ok := reg.Find(want.name)
		if !ok {
			t.Errorf("%s is in spec 15.1 and not in the registry", want.name)
			continue
		}
		if !got.Spec {
			t.Errorf("%s is a row of the spec and is not marked so", want.name)
		}
		if !slices.Equal(got.Then, want.then) {
			t.Errorf("%s: words %v, the spec has %v", want.name, got.Then, want.then)
		}
		if got.Context != want.context {
			t.Errorf("%s: context %s, the spec has %s", want.name, got.Context, want.context)
		}
		if got.EndsInPlan() != want.plan {
			t.Errorf("%s: ends in a plan is %v, the spec has %v", want.name, got.EndsInPlan(), want.plan)
		}
		if !slices.Equal(got.Exits, want.exits) {
			t.Errorf("%s: closing words %v, the spec has %v", want.name, got.Exits, want.exits)
		}
		if got.EndsInPlan() {
			plans++
		}
	}
	if plans != nPlans {
		t.Errorf("the registry marks %d commands as ending in a plan, the spec states %d", plans, nPlans)
	}
	for _, c := range reg.Rows() {
		if !c.Spec && c.Name != "gen" {
			t.Errorf("%s is a row the spec does not name and is not the old gen", c.Name)
		}
	}
}

func TestTheRegistryRefusesGetMakeDestroyAndAWordOffTheBaseList(t *testing.T) {
	t.Parallel()
	good := commands().Rows()
	if _, err := kit.NewRegistry(good); err != nil {
		t.Fatalf("the registry does not hold its own rows: %v", err)
	}
	plant := func(c kit.Command) error {
		_, err := kit.NewRegistry(append(slices.Clone(good), c))
		return err
	}
	base := good[0]
	for _, verb := range kit.Refused() {
		c := base
		c.Name = verb
		c.Examples = [2]kit.Example{{Line: "gragas " + verb, Does: "x"}, {Line: "gragas " + verb + " y", Does: "x"}}
		if err := plant(c); err == nil || !strings.Contains(err.Error(), "refused") {
			t.Errorf("the verb %q was taken: %v", verb, err)
		}
	}
	c := base
	c.Name = "frobnicate"
	c.Examples = [2]kit.Example{{Line: "gragas frobnicate", Does: "x"}, {Line: "gragas frobnicate y", Does: "x"}}
	if err := plant(c); err == nil || !strings.Contains(err.Error(), "base list") {
		t.Errorf("a word off the base list was taken: %v", err)
	}
	if err := plant(base); err == nil || !strings.Contains(err.Error(), "declared twice") {
		t.Errorf("a name declared twice was taken: %v", err)
	}
	g := base
	g.Name, g.Then = "rule", nil
	g.Examples = [2]kit.Example{{Line: "gragas rule", Does: "x"}, {Line: "gragas rule y", Does: "x"}}
	if _, err := kit.NewRegistry([]kit.Command{g}); err == nil || !strings.Contains(err.Error(), "group with no verb") {
		t.Errorf("a group with no verb was taken: %v", err)
	}
	bad := base
	bad.Exits = []model.Word{"maybe"}
	if _, err := kit.NewRegistry([]kit.Command{bad}); err == nil || !strings.Contains(err.Error(), "closing word") {
		t.Errorf("a ninth closing word was taken: %v", err)
	}
	bad = base
	bad.Examples[0].Line = "gragas other"
	if _, err := kit.NewRegistry([]kit.Command{bad}); err == nil {
		t.Error("an example of another command was taken")
	}
}

func TestEveryWordOfTheRegistryIsOnTheBaseListOrAGroup(t *testing.T) {
	t.Parallel()
	for _, c := range commands().Rows() {
		for _, w := range append([]string{c.Name}, c.Then...) {
			if !slices.Contains(kit.BaseVerbs(), w) && !slices.Contains(kit.Groups(), w) {
				t.Errorf("%s is not a verb of the base list", w)
			}
		}
		if slices.Contains(kit.Groups(), c.Name) != (slices.Contains([]string{"rule", "term", "source", "decision", "contract", "plugin"}, c.Name)) {
			t.Errorf("%s: groups are the six of cli.commands.26", c.Name)
		}
	}
	if !slices.IsSorted(kit.BaseVerbs()) {
		t.Error("the base list is kept sorted, so a new verb is one reviewed line")
	}
}

// tree builds the command tree the way a run builds it.
func tree(t *testing.T) *cobra.Command {
	t.Helper()
	return (&session{}).newRoot(t.Context())
}

// TestTheTreeIsTheRegistrysBuiltAndLegacyRowsAndNothingElse: the tree holds
// the rows that are built or legacy, and a command on the not-built list is
// absent from it.
func TestTheTreeIsTheRegistrysBuiltAndLegacyRowsAndNothingElse(t *testing.T) {
	t.Parallel()
	reg := commands()
	if bad := reg.TreeProblems(treeNames(tree(t))); len(bad) > 0 {
		t.Errorf("the tree and the registry disagree: %v", bad)
	}
	if len(reg.NotBuilt()) == 0 {
		t.Fatal("the not-built list is empty, so the plant below proves nothing")
	}
	// Plant a listed verb in the tree: the check fails and names it.
	for _, c := range reg.NotBuilt() {
		planted := tree(t)
		planted.AddCommand(&cobra.Command{Use: c.Name})
		bad := reg.TreeProblems(treeNames(planted))
		if len(bad) != 1 || !strings.Contains(bad[0], c.Name+" is on the not-built list and in the tree") {
			t.Fatalf("a planted %s was not told: %v", c.Name, bad)
		}
	}
	// A command the registry never heard of, and a built row the tree lacks.
	stranger := tree(t)
	stranger.AddCommand(&cobra.Command{Use: "frobnicate"})
	if bad := reg.TreeProblems(treeNames(stranger)); len(bad) != 1 || !strings.Contains(bad[0], "not in the registry") {
		t.Errorf("a stranger in the tree was not told: %v", bad)
	}
	missing := tree(t)
	missing.RemoveCommand(missing.Commands()[0])
	if bad := reg.TreeProblems(treeNames(missing)); len(bad) != 1 || !strings.Contains(bad[0], "not in the tree") {
		t.Errorf("a missing command was not told: %v", bad)
	}
}

func TestTheLegacyRowsAreTheOldCheckAndGen(t *testing.T) {
	t.Parallel()
	var got []string
	for _, c := range commands().Rows() {
		if c.State == kit.Legacy {
			got = append(got, c.Name)
		}
	}
	if !slices.Equal(got, []string{"check", "gen"}) {
		t.Errorf("legacy rows are %v, want check and gen", got)
	}
}

// walk visits every command below c, c included.
func walk(c *cobra.Command, visit func(*cobra.Command)) {
	visit(c)
	for _, child := range c.Commands() {
		walk(child, visit)
	}
}

// flagMeanings lists a flag name used with two types or two meanings in the
// tree under c (cli.commands.11): the persistent flags of the root count for
// every command.
func flagMeanings(c *cobra.Command) []string {
	type meaning struct{ typ, usage, where string }
	seen := map[string]meaning{}
	var out []string
	walk(c, func(cmd *cobra.Command) {
		cmd.Flags().VisitAll(func(f *pflag.Flag) {
			m := meaning{f.Value.Type(), f.Usage, cmd.CommandPath()}
			if prev, ok := seen[f.Name]; ok && (prev.typ != m.typ || prev.usage != m.usage) {
				out = append(out, f.Name+": "+prev.where+" says "+prev.typ+" "+`"`+prev.usage+`"`+", "+m.where+" says "+m.typ+" "+`"`+m.usage+`"`)
			}
			if _, ok := seen[f.Name]; !ok {
				seen[f.Name] = m
			}
		})
	})
	return out
}

func TestAFlagNameHasOneMeaningAndTypeInEveryCommand(t *testing.T) {
	t.Parallel()
	if bad := flagMeanings(tree(t)); len(bad) > 0 {
		t.Errorf("a flag has two meanings: %v", bad)
	}
	planted := tree(t)
	for _, c := range planted.Commands() {
		if c.Name() == "check" {
			c.Flags().String("apply", "", "Something else")
		}
	}
	if bad := flagMeanings(planted); len(bad) == 0 {
		t.Error("a planted flag of two meanings was not told")
	}
}

func TestHelpTheMenuAndTheReadmeTableAreTheRegistrysAsGoldens(t *testing.T) {
	t.Parallel()
	reg := commands()
	configtest.Golden(t, "help-commands.golden", []byte(helpCommands(reg)+"\n"))
	configtest.Golden(t, "readme-table.golden", []byte(readmeTable(reg)))
	// The menu and the help offer nothing that is not in the tree.
	for _, c := range reg.NotBuilt() {
		if strings.Contains(helpCommands(reg), "gragas "+c.Name+" ") || strings.HasSuffix(helpCommands(reg), "gragas "+c.Name) {
			t.Errorf("help lists %s, which is not built", c.Name)
		}
	}
}

// TestTheCommittedDocumentsAreWhatTheRegistryMakes: docs/commands.md, the
// command-tree file and the README's table are the generator's output, byte
// for byte; make gen-check says the same from the program.
func TestTheCommittedDocumentsAreWhatTheRegistryMakes(t *testing.T) {
	t.Parallel()
	readme, err := os.ReadFile(filepath.Join(repoRoot, ReadmeFile))
	if err != nil {
		t.Fatal(err)
	}
	docs, err := Documents(t.Context(), readme)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 3 {
		t.Fatalf("%d documents, want the command table, the tree file and the README", len(docs))
	}
	for _, d := range docs {
		got, err := os.ReadFile(filepath.Join(repoRoot, d.Path))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != string(d.Data) {
			t.Errorf("%s is not what the registry makes: run make gen", d.Path)
		}
	}
	if _, err := Documents(t.Context(), []byte("# no markers\n")); err == nil {
		t.Error("a README with no markers was spliced")
	}
}

// A row with any one of its required fields empty is refused: the guard that
// no exits and no context were once the only things left unplanted.
func TestARowWithAnEmptyFieldIsRefused(t *testing.T) {
	t.Parallel()
	base := commands().Rows()[0]
	empty := map[string]func(*kit.Command){
		"summary":   func(c *kit.Command) { c.Summary = "" },
		"json kind": func(c *kit.Command) { c.JSONKind = "" },
		"exits":     func(c *kit.Command) { c.Exits = nil },
		"context":   func(c *kit.Command) { c.Context = "" },
		"ends":      func(c *kit.Command) { c.Ends = "" },
		"state":     func(c *kit.Command) { c.State = "" },
	}
	for name, blank := range empty {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			c := base
			blank(&c)
			if _, err := kit.NewRegistry([]kit.Command{c}); err == nil || !strings.Contains(err.Error(), "a field is empty") {
				t.Errorf("a row with no %s was taken: %v", name, err)
			}
		})
	}
}

// TestAWordNotBuiltYetIsOnTheListAndOffTheHelpAndTheTree: a family whose last
// word ships after its first lists that word as typed on the not-built list,
// leaves it off the help and the tree, and the README says so. journal and
// cache are built in full now, so the row is a planted one. Risk: a command
// offered before it works (cli.commands.9).
func TestAWordNotBuiltYetIsOnTheListAndOffTheHelpAndTheTree(t *testing.T) {
	t.Parallel()
	rows := commands().Rows()
	for i := range rows {
		if rows[i].Name == "cache" {
			rows[i].NotYet = []string{"clear"}
		}
	}
	reg, err := kit.NewRegistry(rows)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(reg.NotBuiltWords(), "cache clear") {
		t.Errorf("the not-built list is %v, want cache clear on it", reg.NotBuiltWords())
	}
	if strings.Contains(helpCommands(reg), "cache clear") || !strings.Contains(helpCommands(reg), "cache list") {
		t.Errorf("help is\n%s\nwant cache list and not cache clear", helpCommands(reg))
	}
	if !strings.Contains(readmeTable(reg), "yes, except clear") {
		t.Error("the README table does not say clear is not built")
	}
	// A word that is not one of the row's words cannot be marked.
	bad := commands().Rows()
	for i := range bad {
		if bad[i].Name == "cache" {
			bad[i].NotYet = []string{"nonsense"}
		}
	}
	if _, err := kit.NewRegistry(bad); err == nil || !strings.Contains(err.Error(), "nonsense") {
		t.Errorf("a NotYet word outside Then was accepted: %v", err)
	}
}

// persistentHooks are the names of a command's hooks that run for its whole
// subtree, and which of them a command sets.
func persistentHooks(c *cobra.Command) []string {
	var out []string
	for name, set := range map[string]bool{
		"PersistentPreRun": c.PersistentPreRun != nil, "PersistentPreRunE": c.PersistentPreRunE != nil,
		"PersistentPostRun": c.PersistentPostRun != nil, "PersistentPostRunE": c.PersistentPostRunE != nil,
	} {
		if set {
			out = append(out, name)
		}
	}
	slices.Sort(out)
	return out
}

// hookProblems names each command below the root that sets a persistent hook:
// cobra runs only the nearest one, so a command that sets its own replaces the
// root's, which refuses on a settings problem and claims the run record and the
// lock for a run that writes.
func hookProblems(root *cobra.Command) []string {
	var out []string
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		if c != root {
			for _, h := range persistentHooks(c) {
				out = append(out, c.CommandPath()+" sets "+h)
			}
		}
		for _, k := range c.Commands() {
			walk(k)
		}
	}
	walk(root)
	return out
}

// TestNoCommandReplacesTheRootsRunHook: the root's persistent hook is the only
// one in the tree, so every run passes through it. The plant sets one on a
// command of a package and is named.
func TestNoCommandReplacesTheRootsRunHook(t *testing.T) {
	t.Parallel()
	root := tree(t)
	if len(persistentHooks(root)) == 0 {
		t.Fatal("the root sets no persistent hook, so the check proves nothing")
	}
	if bad := hookProblems(root); len(bad) != 0 {
		t.Errorf("commands that replace the root's hook: %v", bad)
	}
	var version *cobra.Command
	for _, c := range root.Commands() {
		if c.Name() == "version" {
			version = c
		}
	}
	if version == nil {
		t.Fatal("no version command to plant on")
	}
	version.PersistentPreRunE = func(*cobra.Command, []string) error { return nil }
	if bad := hookProblems(root); len(bad) != 1 || !strings.Contains(bad[0], "version sets PersistentPreRunE") {
		t.Errorf("a planted hook was not named: %v", bad)
	}
}

// TestJournalAndCacheAreBuiltAndInTheTree: the real registry has journal and
// cache built with every word, none of them on the not-built list, each word
// a command of the tree, and the help offers each example. Risk: a command in
// the tree the registry calls not built, or the reverse (cli.commands.9).
func TestJournalAndCacheAreBuiltAndInTheTree(t *testing.T) {
	t.Parallel()
	reg := commands()
	for _, name := range []string{"journal", "cache"} {
		row, ok := reg.Find(name)
		if !ok || row.State != kit.Built || len(row.NotYet) != 0 {
			t.Errorf("%s: row %+v, want built with no word not built", name, row)
		}
	}
	for _, w := range reg.NotBuiltWords() {
		if strings.HasPrefix(w, "journal") || strings.HasPrefix(w, "cache") {
			t.Errorf("%q is on the not-built list", w)
		}
	}
	for _, path := range [][]string{{"journal", "list"}, {"journal", "recover"}, {"journal", "restore"}, {"cache", "list"}, {"cache", "clear"}} {
		c, _, err := tree(t).Find(path)
		if err != nil || c.Name() != path[1] {
			t.Errorf("%v resolves to %v (%v), want the command", path, c.Name(), err)
		}
	}
	if p := reg.TreeProblems(treeNames(tree(t))); len(p) != 0 {
		t.Errorf("the tree and the registry disagree: %v", p)
	}
}
