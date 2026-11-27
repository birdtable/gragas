// Package gatetest gives the gate's tests the rulebook as the format: the repository's own folder
// when it is in the format, and before the switch a converted copy of it made in a
// folder of the test's own, by the engine's own migration, in this process and with no built program.
// A test imports it from _test.go files only.
package gatetest

import (
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/core/format"
	"github.com/birdtable/gragas/internal/core/link"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/ordered"
)

// Book is a rulebook in the format.
type Book struct {
	// Dir is a folder with the rulebook in its rulebook folder.
	Dir string
	// Rulebook is that folder, as a file system.
	Rulebook fs.FS
	// Statements are the digests a conversion recorded, nil for a rulebook that was already converted.
	Statements map[model.RuleID]model.Digest
}

// Of is the rulebook of the repository at root: the repository's own folder when it holds
// rulebook/rulebook.yaml, and a converted copy of its rulebook/ otherwise.
func Of(t testing.TB, root string) Book {
	t.Helper()
	live := filepath.Join(root, "rulebook")
	if _, err := os.Stat(filepath.Join(live, "rulebook.yaml")); err == nil {
		return Book{Dir: root, Rulebook: os.DirFS(live)}
	}
	dir := t.TempDir()
	copied := filepath.Join(dir, "rulebook")
	if err := os.CopyFS(copied, os.DirFS(live)); err != nil {
		t.Fatal(err)
	}
	v2 := format.V2{}
	var all []string
	if err := fs.WalkDir(os.DirFS(copied), ".", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			all = append(all, p)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	for _, p := range format.V2Files(all) {
		b, err := fs.ReadFile(os.DirFS(copied), p)
		if err != nil {
			t.Fatal(err)
		}
		v2[p] = b
	}
	form := model.InitForm{
		Name: "gragas", Kind: model.KindBase, Version: "1.0.0", Language: "en",
		Licences: model.Licences{Rules: "CC-BY-4.0", Presets: "MIT"}, Owners: []string{"team-a"},
		Governance: model.Governance{Contribution: model.ContributionProposals, DecisionRequired: true, TrialReleases: 3, ReviewDays: 90},
	}
	mig, err := format.MigrateV2(v2, form)
	if err != nil {
		t.Fatal(err)
	}
	for _, tg := range mig.Targets {
		full := filepath.Join(copied, filepath.FromSlash(tg.Path))
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, tg.Data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return Book{Dir: dir, Rulebook: os.DirFS(copied), Statements: mig.Statements}
}

// Shipped is the set of rules the committed, generated presets ship: every rule id named on a line
// between a "generated:<name>:begin" line and its end in a file below presets/<tool>/ that opens
// with the generated mark (link.Mark), which is what "held" means to the register test: what the
// file enables. The plugin that serves the tool writes the file, and preset --check
// holds it byte for byte while it carries the mark, so the lines read here are the generated ones.
// A file a project took over, its mark removed, ships nothing here, and neither does a file of a
// tool the identity lists in hand_written_presets, which preset --check compares none of: the set
// read here is the set that check holds.
func (b Book) Shipped(t testing.TB) map[string]bool {
	t.Helper()
	presets := os.DirFS(filepath.Join(b.Dir, "presets"))
	tools, err := fs.ReadDir(presets, ".")
	if err != nil {
		t.Fatal(err)
	}
	hand := b.handWritten(t)
	out := map[string]bool{}
	for _, tool := range tools {
		if !tool.IsDir() || slices.Contains(hand, tool.Name()) {
			continue
		}
		files, err := fs.ReadDir(presets, tool.Name())
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range files {
			if f.IsDir() {
				continue
			}
			data, err := fs.ReadFile(presets, path.Join(tool.Name(), f.Name()))
			if err != nil {
				t.Fatal(err)
			}
			for _, id := range generatedRules(string(data)) {
				out[id] = true
			}
		}
	}
	return out
}

// handWritten is the tools the book's identity lists in hand_written_presets, none when the book
// has no identity file or lists none.
func (b Book) handWritten(t testing.TB) []string {
	t.Helper()
	data, err := fs.ReadFile(b.Rulebook, format.IdentityFile)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	doc, err := ordered.ParseStrict(data)
	if err != nil {
		t.Fatal(err)
	}
	if m, ok := doc.(*ordered.Map); ok {
		return ordered.Strings(m.List("hand_written_presets"))
	}
	return nil
}

// ruleID is a rule id as a preset's line names it: part.concern.n.
var ruleID = regexp.MustCompile(`^[a-z]+\.[a-z][a-z0-9-]*\.[0-9]+$`)

// generatedRules is the rule ids text names on its generated lines, when its first line carries
// the generated mark: the words of the comment of each line between a "generated:<name>:begin"
// line and the next "generated:<name>:end", in the order of the file. Text with no mark names none,
// and so does a line outside a block or a block whose end never comes: what the register test
// counts as held is what the plugin wrote, never more.
func generatedRules(text string) []string {
	lines := strings.Split(text, "\n")
	if !strings.Contains(lines[0], link.Mark()) {
		return nil
	}
	var out, block []string
	end := ""
	for _, l := range lines[1:] {
		l = strings.TrimSpace(l)
		switch {
		case end == "":
			end = endOf(l)
		case l == end:
			out, block, end = append(out, block...), nil, ""
		default:
			block = append(block, commentRules(l)...)
		}
	}
	return out
}

// endOf is the line that ends the generated block l begins, or "" when l begins none.
func endOf(l string) string {
	name, ok := strings.CutPrefix(l, "# generated:")
	if !ok {
		return ""
	}
	if name, ok = strings.CutSuffix(name, ":begin"); !ok {
		return ""
	}
	return "# generated:" + name + ":end"
}

// commentRules is the rule ids the comment of line l names, in its order.
func commentRules(l string) []string {
	_, comment, ok := strings.Cut(l, "#")
	if !ok {
		return nil
	}
	var out []string
	for _, w := range strings.FieldsFunc(comment, func(r rune) bool { return r == ',' || r == ' ' || r == '(' || r == ')' }) {
		if ruleID.MatchString(w) {
			out = append(out, w)
		}
	}
	return out
}
