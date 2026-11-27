// Package newcommand makes the files of a new command from the golden
// example: its package below internal/cli/cmd, the operation of the core, the
// test stubs and the journey stub. It reads the tree and the templates through
// fs.FS and returns what to write; it writes nothing, so the generator's main
// is the one place that touches the disk. A new command is a new folder and
// the files task gen writes from it (join_gen.go): it edits no hand-written
// file, and the plan is refused when it names one outside its own package.
package newcommand

import (
	"bytes"
	"errors"
	"fmt"
	"go/format"
	"io/fs"
	"regexp"
	"slices"
	"strings"
	"text/template"
)

// Words are the lists a name is held to. They come from the command registry,
// so the generator refuses a name by the registry's own rules.
type Words struct {
	// Base is the one list every command word comes from.
	Base []string
	// Groups take a verb; the rest are flat.
	Groups []string
	// Refused are the verbs the registry never takes.
	Refused []string
}

// Name is what is being made: a group and a verb, or a flat verb alone.
type Name struct {
	Group, Verb string
}

// String is the words as typed after gragas.
func (n Name) String() string { return strings.TrimSpace(n.Group + " " + n.Verb) }

// Family is the folder below internal/cli/cmd the command lives in: its group,
// or its verb when it is flat.
func (n Name) Family() string {
	if n.Group != "" {
		return n.Group
	}
	return n.Verb
}

// fields are what the templates read.
type fields struct {
	Group, Verb, Name, Family string
	// File is the file stem of the core's files, Camel the method name, Pascal the test name.
	File, Camel, Pascal string
	// The row: Order, the context and ending as the kit's constants, the
	// summary, the two examples, the files written, the closing words as the
	// model's constants, the kind of the --json document and the words after the name.
	Order                          int
	Context, Ends                  string
	OptIn                          bool
	Summary                        string
	Examples                       [2]struct{ Line, Does string }
	Writes, Exits                  string
	JSON                           string
	Then, NotYet                   string
	HasThen, HasNotYet, NewPackage bool
}

func fieldsOf(n Name, s Seed, then []string) (fields, error) {
	parts := []string{}
	if n.Group != "" {
		parts = append(parts, n.Group)
	}
	parts = append(parts, n.Verb)
	var b strings.Builder
	b.WriteString(parts[0])
	for _, p := range parts[1:] {
		b.WriteString(strings.ToUpper(p[:1]) + p[1:])
	}
	camel := b.String()
	f := fields{Group: n.Group, Verb: n.Verb, Name: n.String(), Family: n.Family(), File: strings.Join(parts, "_"),
		Camel: camel, Pascal: strings.ToUpper(camel[:1]) + camel[1:],
		Order: s.Order, OptIn: s.OptIn, Summary: s.Summary, JSON: s.JSON, Writes: quoted(s.Writes), Then: quoted(then)}
	for i, e := range s.Examples {
		f.Examples[i].Line, f.Examples[i].Does = e[0], e[1]
	}
	f.HasThen = len(then) > 0
	if n.Group != "" {
		f.NotYet = quoted(slices.DeleteFunc(slices.Clone(then), func(v string) bool { return v == n.Verb }))
		f.HasNotYet = f.NotYet != ""
	}
	var err error
	if f.Context, err = named(contextNames(), "context", []string{s.Context}); err != nil {
		return fields{}, err
	}
	if f.Ends, err = named(endingNames(), "ending", []string{s.Ends}); err != nil {
		return fields{}, err
	}
	if f.Exits, err = named(closingNames(), "closing word", s.Exits); err != nil {
		return fields{}, err
	}
	return f, nil
}

// Paths of the folders a command is written in.
const (
	cmdDir     = "internal/cli/cmd/"
	plannedDir = "internal/cli/planned/"
	apiDir     = "internal/core/api/"
	journeyDir = "internal/journeys/"
)

// targets is where each template lands. The package file is written only when
// the package is new; a verb of a group has a file of its own in it.
func targets(n Name, newPackage bool) map[string]string {
	pkg := cmdDir + n.Family() + "/"
	out := map[string]string{
		"api.go.tmpl":          apiDir + "{File}.go",
		"unit_test.go.tmpl":    apiDir + "{File}_test.go",
		"journey_test.go.tmpl": journeyDir + "{File}_test.go",
	}
	if newPackage {
		out["command.go.tmpl"] = pkg + "command.go"
	}
	if n.Group == "" {
		out["golden_test.go.tmpl"] = pkg + "command_test.go"
	} else {
		out["verb.go.tmpl"] = pkg + n.Verb + ".go"
		out["golden_test.go.tmpl"] = pkg + n.Verb + "_test.go"
	}
	return out
}

// Plan is what a run would write.
type Plan struct {
	// Files are new files, by path from the root.
	Files map[string]string
	// Removes are files that go: the planned row of the command, now built.
	Removes []string
	// Note is what the command's author still does by hand, said after the write.
	Note string
}

// Paths are the files the plan writes and removes, sorted.
func (p Plan) Paths() []string {
	var out []string
	for path := range p.Files {
		out = append(out, path)
	}
	for _, path := range p.Removes {
		out = append(out, path+" (removed: the command is built)")
	}
	slices.Sort(out)
	return out
}

var wordRe = regexp.MustCompile(`^[a-z]+$`)

// Refuse says why a name may not be made, nil when it may. taken holds the
// names the registry already declares as built, with the verbs of each group.
func Refuse(w Words, n Name, taken map[string][]string) error {
	var errs []error
	for _, word := range []string{n.Group, n.Verb} {
		switch {
		case word == "":
		case !wordRe.MatchString(word):
			errs = append(errs, fmt.Errorf("%q is not a word of lower-case letters", word))
		case slices.Contains(w.Refused, word):
			errs = append(errs, fmt.Errorf("the verb %q is refused", word))
		case !slices.Contains(w.Base, word) && !slices.Contains(w.Groups, word):
			errs = append(errs, fmt.Errorf("%q is not on the base list of verbs", word))
		}
	}
	if n.Verb == "" {
		return errors.Join(append(errs, errors.New("a command has a verb"))...)
	}
	if n.Group == "" {
		if slices.Contains(w.Groups, n.Verb) {
			errs = append(errs, fmt.Errorf("%q is a group and takes a verb", n.Verb))
		}
		if _, ok := taken[n.Verb]; ok {
			errs = append(errs, fmt.Errorf("%q is already declared", n.Verb))
		}
		return errors.Join(errs...)
	}
	if !slices.Contains(w.Groups, n.Group) {
		errs = append(errs, fmt.Errorf("%q is not a group: the groups are %s", n.Group, strings.Join(w.Groups, ", ")))
	} else if slices.Contains(taken[n.Group], n.Verb) {
		errs = append(errs, fmt.Errorf("%s is already declared", n))
	}
	return errors.Join(errs...)
}

// render is the files the templates make for a name from a seed, by path from
// the root: the .go files are formatted.
func render(tmpl fs.FS, n Name, s Seed, then []string, newPackage bool) (map[string]string, error) {
	out := map[string]string{}
	f, err := fieldsOf(n, s, then)
	if err != nil {
		return nil, err
	}
	f.NewPackage = newPackage
	for name, target := range targets(n, newPackage) {
		src, err := fs.ReadFile(tmpl, "files/"+name)
		if err != nil {
			return nil, fmt.Errorf("template %s: %w", name, err)
		}
		t, err := template.New(name).Option("missingkey=error").Parse(string(src))
		if err != nil {
			return nil, fmt.Errorf("template %s: %w", name, err)
		}
		var b bytes.Buffer
		if err := t.Execute(&b, f); err != nil {
			return nil, fmt.Errorf("template %s: %w", name, err)
		}
		text := b.Bytes()
		if strings.HasSuffix(target, ".go") {
			if text, err = format.Source(text); err != nil {
				return nil, fmt.Errorf("template %s makes no Go: %w", name, err)
			}
		}
		out[strings.ReplaceAll(target, "{File}", f.File)] = string(text)
	}
	return out, nil
}

// Render is the files the templates make for a name with the stub row, in a
// package that is new: what the golden example holds.
func Render(tmpl fs.FS, n Name) (map[string]string, error) {
	return render(tmpl, n, stub(n), thenOf(n, nil), true)
}

// thenOf are the words after the name: the verb alone, or the planned ones with it.
func thenOf(n Name, planned []string) []string {
	if n.Group == "" {
		return planned
	}
	if slices.Contains(planned, n.Verb) {
		return planned
	}
	return append(slices.Clone(planned), n.Verb)
}

// Make is the plan for a name the caller has already held to Refuse. tmpl
// holds the templates, tree the repository, planned the seed of the command's
// planned row when it has one; no file it makes may exist. The plan removes
// that planned row and edits nothing else.
func Make(tmpl, tree fs.FS, n Name, planned *Seed) (Plan, error) {
	pkg := strings.TrimSuffix(cmdDir, "/") + "/" + n.Family()
	_, err := fs.Stat(tree, pkg)
	newPackage := err != nil
	seed := stub(n)
	if planned != nil {
		seed = *planned
	}
	switch {
	case n.Group != "" && newPackage && planned == nil:
		return Plan{}, fmt.Errorf("%s has no package and no planned file: its commands are still in the old table of internal/cli/root, and move into %s first", n.Group, pkg)
	case n.Group != "" && !newPackage:
		planned = nil
	}
	files, err := render(tmpl, n, seed, thenOf(n, seed.Then), newPackage)
	if err != nil {
		return Plan{}, err
	}
	plan := Plan{Files: files}
	if planned != nil {
		plan.Removes = []string{plannedDir + n.Family() + ".yaml"}
		if _, err := fs.Stat(tree, plan.Removes[0]); err != nil {
			return Plan{}, fmt.Errorf("%s: %w", plan.Removes[0], err)
		}
	}
	if n.Group != "" && !newPackage {
		plan.Note = "the package " + pkg + " exists: add " + n.Verb + " to its Row (Then, and out of NotYet) and to its New by hand"
	}
	for path := range plan.Files {
		if _, err := fs.Stat(tree, path); err == nil {
			return Plan{}, fmt.Errorf("%s exists: nothing is overwritten", path)
		}
	}
	return plan, Outside(n, plan)
}

// Outside is the paths of a plan that are not the command's own: a hand-written
// file elsewhere in the tree, which a new command never touches (it is a new
// folder, the core's operation and test files of the same name, and the planned
// row it replaces; the rest is generated by task gen). An error names each.
func Outside(n Name, p Plan) error {
	own := func(path string) bool {
		if !fs.ValidPath(path) {
			return false
		}
		stem := strings.ReplaceAll(n.String(), " ", "_")
		return strings.HasPrefix(path, cmdDir+n.Family()+"/") ||
			path == apiDir+stem+".go" || path == apiDir+stem+"_test.go" || path == journeyDir+stem+"_test.go"
	}
	var bad []string
	for path := range p.Files {
		if !own(path) {
			bad = append(bad, path)
		}
	}
	for _, path := range p.Removes {
		if path != plannedDir+n.Family()+".yaml" {
			bad = append(bad, path)
		}
	}
	slices.Sort(bad)
	if len(bad) > 0 {
		return fmt.Errorf("the plan touches %s, which are not the new package's: a command changes no hand-written file outside it", strings.Join(bad, ", "))
	}
	return nil
}

// Drift lists the golden files the templates no longer render for the sample
// name, and the golden files no template renders. Nothing when they agree.
func Drift(tmpl, golden fs.FS, sample Name) ([]string, error) {
	want, err := Render(tmpl, sample)
	if err != nil {
		return nil, err
	}
	var out []string
	err = fs.WalkDir(golden, "golden", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel := strings.TrimSuffix(strings.TrimPrefix(path, "golden/"), ".golden")
		got, err := fs.ReadFile(golden, path)
		if err != nil {
			return err
		}
		text, ok := want[rel]
		switch {
		case !ok:
			out = append(out, rel+" is a golden file no template renders")
		case text != string(got):
			out = append(out, rel+" differs from what the templates render")
		}
		delete(want, rel)
		return nil
	})
	if err != nil {
		return nil, err
	}
	for path := range want {
		out = append(out, path+" has no golden file")
	}
	slices.Sort(out)
	return out, nil
}
