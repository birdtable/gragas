package coreapi_test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/core/audit"
	"github.com/birdtable/gragas/internal/core/format"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// trackedAudit is a repository whose index holds files and links, and whose
// work tree holds work.
// linterCounting is the count's data of the test repositories' linter, as its
// plugin gives it: the test hands it in, and the operations name
// no tool's paths.
func linterCounting() []coreapi.Counting {
	return []coreapi.Counting{{
		Configs: []string{".golangci.yml"},
		Exclusions: [][]string{
			{"linters", "exclusions", "rules"}, {"linters", "exclusions", "paths"},
			{"formatters", "exclusions", "paths"}, {"linters", "exclusions", "presets"},
			{"issues", "exclude-rules"}, {"issues", "exclude"}, {"issues", "exclude-dirs"}, {"issues", "exclude-files"},
			{"run", "skip-dirs"}, {"run", "skip-files"},
		},
		Loosening: []coreapi.Loosening{
			{At: []string{"linters", "exclusions", "generated"}, Unless: new("strict")},
			{At: []string{"linters", "exclusions", "paths-except"}, Unless: new("[]")},
			{At: []string{"run", "tests"}, Unless: new("true")},
			{At: []string{"run", "issues-exit-code"}, Unless: new("1")},
			{At: []string{"issues", "new"}, Unless: new("false")},
			{At: []string{"issues", "new-from-rev"}, Unless: new("")},
			{At: []string{"issues", "new-from-merge-base"}, Unless: new("")},
			{At: []string{"issues", "new-from-patch"}, Unless: new("")},
		},
		Directive: "nolint",
	}}
}

func trackedAudit(t *testing.T, files map[string]string, links map[string]string) coreapi.RepoAudit {
	t.Helper()
	w := porttest.NewWorld(t)
	root := w.Seed("repo", map[string]string{"x": "y"})
	w.SeedRepo(root, porttest.Repo{Index: files, IndexLinks: links,
		Bases: map[[2]string]string{{"origin/main", "HEAD"}: "c0"},
		Heads: map[string]porttest.Rev{"base": {Commit: "c0", Files: files}}})
	work := fstest.MapFS{}
	for p, s := range files {
		work[p] = &fstest.MapFile{Data: []byte(s)}
	}
	return coreapi.RepoAudit{History: w.Ports().History, Root: root, Src: work, Rules: rules(), FileNames: w.Ports().FileNames}
}

// rules is the rule each key is checked under, as a rulebook would say.
func rules() map[string]string {
	return map[string]string{audit.KeyLink: "t.link", audit.KeyFold: "t.fold", audit.KeyNames: "t.names", audit.KeyDebt: "t.debt", audit.KeyDirective: "t.directive"}
}

func trackedOnly(r coreapi.RepoFindings) []coreapi.ConfigFinding {
	var out []coreapi.ConfigFinding
	for _, f := range r.Findings {
		if f.Key == audit.KeyLink || f.Key == audit.KeyFold {
			out = append(out, f)
		}
	}
	return out
}

// TestACleanRepositoryHasNoTrackedFinding is the control: no link and no two
// paths that fold to one.
func TestACleanRepositoryHasNoTrackedFinding(t *testing.T) {
	t.Parallel()
	a := trackedAudit(t, map[string]string{"a.md": "a", "b.md": "b"}, nil)
	if got := trackedOnly(coreapi.CheckRepo(context.Background(), a)); len(got) != 0 {
		t.Errorf("findings %v on a clean repository", got)
	}
}

// TestATrackedLinkAndACaseFoldAreFindings is the plant.
func TestATrackedLinkAndACaseFoldAreFindings(t *testing.T) {
	t.Parallel()
	a := trackedAudit(t, map[string]string{"a.md": "a", "A.md": "b"}, map[string]string{"CONTRIBUTING.md": "a.md"})
	if got := trackedOnly(coreapi.CheckRepo(context.Background(), a)); len(got) != 2 {
		t.Errorf("findings %v, want the link and the fold", got)
	}
}

// TestATrackedNameNoSystemCanHoldIsNotCheckedAndNeverAFinding: the keys of the tracked paths
// come from the tree's Portable, each name of a path asked in turn. A name the tree refuses, as v1. and CON are, is a
// not-checked row of the tracked-files check naming the path and the reason, no finding of the run and no clean
// answer for that path, and the case pair beside it is still a finding.
// Risk: a tracked name no disk holds is dropped from the run, or a rule is made for it.
func TestATrackedNameNoSystemCanHoldIsNotCheckedAndNeverAFinding(t *testing.T) {
	t.Parallel()
	a := trackedAudit(t, map[string]string{"a.md": "a", "A.md": "b", "dir/v1.": "c", "CON": "d"}, nil)
	r := coreapi.CheckRepo(context.Background(), a)
	if got := trackedOnly(r); len(got) != 1 || !strings.Contains(got[0].Missing, "differs only by case") {
		t.Errorf("findings %v, want the one case pair", got)
	}
	var said []string
	for _, n := range r.NotChecked {
		if strings.Contains(n.Tool, audit.KeyTracked) {
			said = append(said, n.Why)
		}
	}
	all := strings.Join(said, "\n")
	for _, want := range []string{"dir/v1. was not checked", "CON was not checked", "not a portable name"} {
		if !strings.Contains(all, want) {
			t.Errorf("no %q in the not-checked rows:\n%s", want, all)
		}
	}
}

// TestATwinBelowAFolderThePortRefusesIsStillOneFinding: aux
// is a name the port refuses, and aux/Readme.md and aux/README.md are one finding on release-1; they stay one, the
// refused folder being the same name on both sides and the names below it compared. The refused name is still a
// not-checked row, never a clean answer for it.
// Risk: this change turns a finding into not-checked rows, and a case pair below a refused folder goes unseen.
func TestATwinBelowAFolderThePortRefusesIsStillOneFinding(t *testing.T) {
	t.Parallel()
	a := trackedAudit(t, map[string]string{"aux/Readme.md": "a", "aux/README.md": "b", "notes/a.md": "c"}, nil)
	r := coreapi.CheckRepo(context.Background(), a)
	if got := trackedOnly(r); len(got) != 1 || !strings.Contains(got[0].Missing, "differs only by case") {
		t.Errorf("findings %v, want the one case pair below aux", got)
	}
	var said []string
	for _, n := range r.NotChecked {
		if strings.Contains(n.Tool, audit.KeyTracked) {
			said = append(said, n.Why)
		}
	}
	if all := strings.Join(said, "\n"); !strings.Contains(all, "aux/Readme.md") || !strings.Contains(all, "not a portable name") {
		t.Errorf("the refused folder is not a not-checked row naming the path and the reason:\n%s", all)
	}
}

// TestNoGitLeavesTheThreeChecksUnjudged is the refusal: nothing passes for
// want of a repository.
func TestNoGitLeavesTheThreeChecksUnjudged(t *testing.T) {
	t.Parallel()
	got := coreapi.CheckRepo(context.Background(), coreapi.RepoAudit{NoGit: true})
	if len(got.NotChecked) != 3 || len(got.Findings) != 0 {
		t.Errorf("%+v, want three unjudged and no finding", got)
	}
}

// TestANewDirectiveAgainstTheMergeBaseIsAFinding: the plant adds a nolint
// line to a file the merge base has without one; the control is the same file
// unchanged.
func TestANewDirectiveAgainstTheMergeBaseIsAFinding(t *testing.T) {
	t.Parallel()
	was := "package p\n\nfunc f() { g() }\n"
	now := "package p\n\nfunc f() { g() } //nolint:errcheck\n"
	w := porttest.NewWorld(t)
	root := w.Seed("repo", map[string]string{"x": "y"})
	w.SeedRepo(root, porttest.Repo{Index: map[string]string{"a.go": now},
		Bases: map[[2]string]string{{"origin/main", "HEAD"}: "c0"},
		Heads: map[string]porttest.Rev{"base": {Commit: "c0", Files: map[string]string{"a.go": was}}}})
	a := coreapi.RepoAudit{History: w.Ports().History, FileNames: w.Ports().FileNames, Root: root, Rules: rules(), Counting: linterCounting(),
		Src: fstest.MapFS{"a.go": &fstest.MapFile{Data: []byte(now)}}}
	var got []coreapi.ConfigFinding
	for _, f := range coreapi.CheckRepo(context.Background(), a).Findings {
		if f.Key == audit.KeyDirective {
			got = append(got, f)
		}
	}
	if len(got) != 1 || got[0].File != "a.go" || got[0].Line != 3 {
		t.Errorf("findings %v, want the one new directive at a.go:3", got)
	}
	w2 := porttest.NewWorld(t)
	root2 := w2.Seed("repo", map[string]string{"x": "y"})
	w2.SeedRepo(root2, porttest.Repo{Index: map[string]string{"a.go": was},
		Bases: map[[2]string]string{{"origin/main", "HEAD"}: "c0"},
		Heads: map[string]porttest.Rev{"base": {Commit: "c0", Files: map[string]string{"a.go": was}}}})
	a.History, a.Root, a.Src = w2.Ports().History, root2, fstest.MapFS{"a.go": &fstest.MapFile{Data: []byte(was)}}
	for _, f := range coreapi.CheckRepo(context.Background(), a).Findings {
		t.Errorf("control: finding %v on an unchanged file", f)
	}
}

// TestAFileOverTheReadCapIsLeftUnreadAndOneAtItIsRead holds the cap on one
// file the audit reads: 8 MiB is read, one byte more is said to be left
// unread and never read in part.
// boundary: maxRead
// Risk: a whole repository's worth of one file held in memory by a check.
func TestAFileOverTheReadCapIsLeftUnreadAndOneAtItIsRead(t *testing.T) {
	t.Parallel()
	const limit = 8 << 20
	for _, tc := range []struct {
		name string
		size int
		want int
	}{{"at the cap", limit, 0}, {"over the cap", limit + 1, 1}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			big := string(make([]byte, tc.size))
			a := trackedAudit(t, map[string]string{"big.txt": big}, nil)
			a.Rules = rules()
			unread := 0
			for _, n := range coreapi.CheckRepo(context.Background(), a).NotChecked {
				if strings.Contains(n.Why, "was not read") {
					unread++
				}
			}
			if unread != tc.want {
				t.Errorf("%d files said to be unread at %d bytes, want %d", unread, tc.size, tc.want)
			}
		})
	}
}

// notChecked is the not-checked rows of a run that say key.
func notChecked(r coreapi.RepoFindings, key string) []coreapi.ConfigNotChecked {
	var out []coreapi.ConfigNotChecked
	for _, n := range r.NotChecked {
		if n.Tool == audit.Program+" "+key {
			out = append(out, n)
		}
	}
	return out
}

// TestNoRunSaysAnAgeRow holds the retirement: supply.age.1 and
// supply.age.2 are gone, so a run says no module-age row, as a not-checked
// row or as a finding.
func TestNoRunSaysAnAgeRow(t *testing.T) {
	t.Parallel()
	got := coreapi.CheckRepo(context.Background(), trackedAudit(t, map[string]string{"a.md": "a"}, nil))
	if rows := notChecked(got, "age"); len(rows) != 0 {
		t.Errorf("age rows %v, want none", rows)
	}
	for _, f := range got.Findings {
		if f.Key == "age" {
			t.Errorf("age is a finding: %v", f)
		}
	}
}

// TestAnEmptyTrackedListIsNotCheckedAndNotClean: git answers no file at all,
// which is no answer to judge by.
func TestAnEmptyTrackedListIsNotCheckedAndNotClean(t *testing.T) {
	t.Parallel()
	got := coreapi.CheckRepo(context.Background(), trackedAudit(t, map[string]string{}, nil))
	for _, k := range []string{audit.KeyNames, audit.KeyTracked, audit.KeyCounts} {
		if len(notChecked(got, k)) != 1 {
			t.Errorf("%s is not said to be unchecked on an empty list: %v", k, got.NotChecked)
		}
	}
	if len(got.Findings) != 0 {
		t.Errorf("findings %v on an empty list", got.Findings)
	}
}

// TestNoMergeBaseLeavesTheCountsNotCheckedWithTheReason: the history shares no
// commit with the base, so the counts have nothing to compare with.
func TestNoMergeBaseLeavesTheCountsNotCheckedWithTheReason(t *testing.T) {
	t.Parallel()
	w := porttest.NewWorld(t)
	root := w.Seed("repo", map[string]string{"x": "y"})
	w.SeedRepo(root, porttest.Repo{Index: map[string]string{"a.go": "package p\n"}, Bases: map[[2]string]string{{"origin/main", "HEAD"}: ""}})
	a := coreapi.RepoAudit{History: w.Ports().History, FileNames: w.Ports().FileNames, Root: root, Rules: rules(), Src: fstest.MapFS{"a.go": &fstest.MapFile{Data: []byte("package p\n")}}}
	rows := notChecked(coreapi.CheckRepo(context.Background(), a), audit.KeyCounts)
	if len(rows) != 1 || !strings.Contains(rows[0].Why, "merge base") {
		t.Errorf("counts rows %v, want one that says the merge base was not found", rows)
	}
}

// keptLength and directive are a file at the merge base and the file with a
// directive in place of a comment of its length: the two hold as many bytes,
// so a check that took equal byte counts as unchanged would pass the
// directive.
const (
	keptLength = "package p\n\nfunc f() { g() } // aaaaaaaaaaaaaa\n"
	directive  = "package p\n\nfunc f() { g() } //nolint:errcheck\n"
)

// idHistory is a countingHistory whose listings and work-tree ids a test may
// shape: files, when set, changes each listing's entries, and work, when
// set, answers WorkObjects in its place.
type idHistory struct {
	countingHistory
	files func(e []model.Entry) []model.Entry
	work  func(paths []string) ([]string, error)
}

func (h *idHistory) Files(ctx context.Context, root model.Root, rev string) ([]model.Entry, error) {
	e, err := h.countingHistory.Files(ctx, root, rev)
	if err == nil && h.files != nil {
		e = h.files(e)
	}
	return e, err
}

func (h *idHistory) WorkObjects(ctx context.Context, root model.Root, paths []string) ([]string, error) {
	if h.work != nil {
		return h.work(paths)
	}
	return h.countingHistory.WorkObjects(ctx, root, paths)
}

// countsAudit is a repository whose a.go is base at the merge base, index
// in the index, and work in the work tree, which Src and the world's tree
// both hold; and its History, which counts the reads of the merge base.
func countsAudit(t *testing.T, base, index, work string) (coreapi.RepoAudit, *idHistory) {
	t.Helper()
	w := porttest.NewWorld(t)
	root := w.Seed("repo", map[string]string{"a.go": work})
	w.SeedRepo(root, porttest.Repo{Index: map[string]string{"a.go": index},
		Bases: map[[2]string]string{{"origin/main", "HEAD"}: "c0"},
		Heads: map[string]porttest.Rev{"base": {Commit: "c0", Files: map[string]string{"a.go": base}}}})
	h := &idHistory{History: w.Ports().History}
	return coreapi.RepoAudit{History: h, FileNames: w.Ports().FileNames, Root: root, Rules: rules(), Counting: linterCounting(),
		Src: fstest.MapFS{"a.go": &fstest.MapFile{Data: []byte(work)}}}, h
}

// directives is the directive findings of a run of a.
func directives(a coreapi.RepoAudit) []coreapi.ConfigFinding {
	var got []coreapi.ConfigFinding
	for _, f := range coreapi.CheckRepo(context.Background(), a).Findings {
		if f.Key == audit.KeyDirective {
			got = append(got, f)
		}
	}
	return got
}

// TestADirectiveAddedAtTheSameSizeIsAFinding: a staged
// directive that keeps the file's byte count to the byte is a finding,
// since the file's object id at the merge base is not the id of its bytes
// now. This was release 1's named limit, which took equal byte counts as
// unchanged. Planted failure: the base and work ids compared by length.
func TestADirectiveAddedAtTheSameSizeIsAFinding(t *testing.T) {
	t.Parallel()
	if len(keptLength) != len(directive) {
		t.Fatalf("the plant must keep the byte count: %d and %d", len(keptLength), len(directive))
	}
	a, _ := countsAudit(t, keptLength, directive, directive)
	if got := directives(a); len(got) != 1 || got[0].File != "a.go" || got[0].Line != 3 {
		t.Errorf("findings %v, want the one new directive at a.go:3", got)
	}
}

// TestAnUnstagedEditOfEqualLengthIsAFinding: the index holds
// the merge base's a.go, so the index's id is the base's, and the work tree
// holds the directive at the same byte count, not staged. "Now" is the work
// tree, so the directive is a finding. Planted failure: the index's id taken
// for the work tree's.
func TestAnUnstagedEditOfEqualLengthIsAFinding(t *testing.T) {
	t.Parallel()
	a, _ := countsAudit(t, keptLength, keptLength, directive)
	if got := directives(a); len(got) != 1 || got[0].File != "a.go" || got[0].Line != 3 {
		t.Errorf("findings %v, want the one unstaged directive at a.go:3", got)
	}
}

// TestAFileWhoseIdsAgreeIsNotReadAtTheMergeBase: a file whose
// bytes now are its merge base's has one object id, so its base is not read
// and it is no finding. Planted failure: every counted file read.
func TestAFileWhoseIdsAgreeIsNotReadAtTheMergeBase(t *testing.T) {
	t.Parallel()
	a, h := countsAudit(t, directive, directive, directive)
	if got := directives(a); len(got) != 0 || h.blobs != 0 || len(h.asked) != 0 {
		t.Errorf("findings %v, %d reads and %v of the merge base; want none: the ids agree", got, h.blobs, h.asked)
	}
}

// TestAnEntryWithNoObjectIdIsRead: an id that cannot be
// compared never lets a file through. With no id at the merge base, a hash
// that fails, or an answer short of one id a path, the file is read at the
// merge base: an unchanged file is one read and no finding, and a changed
// one is the finding. Planted failure: two empty ids taken as equal.
func TestAnEntryWithNoObjectIdIsRead(t *testing.T) {
	t.Parallel()
	noID := func(e []model.Entry) []model.Entry {
		for i := range e {
			e[i].Object = ""
		}
		return e
	}
	for _, tc := range []struct {
		name  string
		files func([]model.Entry) []model.Entry
		work  func([]string) ([]string, error)
	}{
		{"no id in the listings", noID, nil},
		{"no id anywhere", noID, func(p []string) ([]string, error) { return make([]string, len(p)), nil }},
		{"a hash that fails", nil, func([]string) ([]string, error) { return nil, errors.New("git hash-object failed") }},
		{"an answer short of one id a path", nil, func([]string) ([]string, error) { return []string{}, nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a, h := countsAudit(t, directive, directive, directive)
			h.files, h.work = tc.files, tc.work
			if got := directives(a); len(got) != 0 || h.blobs != 1 {
				t.Errorf("unchanged: findings %v and %d reads of the merge base; want none and one", got, h.blobs)
			}
			a, h = countsAudit(t, keptLength, keptLength, directive)
			h.files, h.work = tc.files, tc.work
			if got := directives(a); len(got) != 1 || got[0].Line != 3 || h.blobs != 1 {
				t.Errorf("changed: findings %v and %d reads of the merge base; want the directive at a.go:3 and one", got, h.blobs)
			}
		})
	}
}

// TestAListTheOwnedReadRefusedLeavesTheNamesCheckNotCheckedAndAnEmptyOneToo
// holds the two ways the list can fail to judge: handed a refusal, or handed
// a list with no name in it. Neither is a pass, and neither says what the
// refused read was.
func TestAListTheOwnedReadRefusedLeavesTheNamesCheckNotCheckedAndAnEmptyOneToo(t *testing.T) {
	t.Parallel()
	empty, err := format.ParseNames([]byte("# nothing here\n"))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		names *format.Names
		err   error
		why   string
	}{
		{"a refused read", nil, errors.New("not owned by this account"), "could not be read"},
		{"a refusal that carries the path", nil, fmt.Errorf("read the names file /data/alice/names.txt: %w", port.ErrNotOwned), "not owned by this account"},
		{"an empty list", empty, nil, "empty"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a := trackedAudit(t, map[string]string{"a.md": "text"}, nil)
			if tc.names != nil {
				a.Names = tc.names
			}
			a.NamesErr = tc.err
			got := coreapi.CheckRepo(context.Background(), a)
			rows := notChecked(got, audit.KeyNames)
			if len(rows) != 1 || !strings.Contains(rows[0].Why, tc.why) {
				t.Errorf("names rows %v, want one that says %q", rows, tc.why)
			}
			for _, f := range got.Findings {
				if f.Key == audit.KeyNames {
					t.Errorf("a names finding %v with no list to judge by", f)
				}
			}
			if strings.Contains(rows[0].Why, "alice") || strings.Contains(rows[0].Why, "/data") {
				t.Errorf("the row says where the list lies: %q", rows[0].Why)
			}
		})
	}
}

// TestATextOutsideTheRepositoryIsReadAndScanned: a file the repository does not
// track is read by ReadOwned and its names found, and the ones that cannot
// be read are named, never passed.
func TestATextOutsideTheRepositoryIsReadAndScanned(t *testing.T) {
	t.Parallel()
	w := porttest.NewWorld(t)
	root := w.Seed("repo", map[string]string{"x": "y"})
	w.SeedRepo(root, porttest.Repo{Index: map[string]string{"a.md": "a"},
		Bases: map[[2]string]string{{"origin/main", "HEAD"}: "c0"},
		Heads: map[string]porttest.Rev{"base": {Commit: "c0", Files: map[string]string{"a.md": "a"}}}})
	w.SeedOutside("/data/x/note.txt", "the word secretname is here\n")
	w.SeedRefused("/data/x/shared.txt", porttest.Refused{Why: "another account can write it", Fix: "chmod go-w it"})
	names, err := format.ParseNames([]byte("secretname\n"))
	if err != nil {
		t.Fatal(err)
	}
	a := coreapi.RepoAudit{History: w.Ports().History, FileNames: w.Ports().FileNames, Root: root, Rules: rules(), Names: names,
		Src: fstest.MapFS{"a.md": &fstest.MapFile{Data: []byte("a")}}, Owned: w.Ports().Owned,
		Texts: []string{"/data/x/note.txt", "/data/x/missing.txt", "relative.txt", "/data/../x", "/data/x/shared.txt"}}
	got := coreapi.CheckRepo(context.Background(), a)
	var hit bool
	for _, f := range got.Findings {
		if f.Key == audit.KeyNames && f.File == "given text 1" {
			hit = true
		}
	}
	if !hit {
		t.Errorf("no name found in the outside text: %v", got.Findings)
	}
	rows := notChecked(got, audit.KeyNames)
	if len(rows) != 4 {
		t.Errorf("names rows %v, want the missing, the relative, the unclean and the refused path", rows)
	}
	refused := func(r coreapi.ConfigNotChecked) bool {
		return strings.Contains(r.Why, "given text 5 was not read: it is not owned by this account")
	}
	if !slices.ContainsFunc(rows, refused) {
		t.Errorf("names rows %v: none says given text 5 is not owned", rows)
	}
}

// refusedTextAudit is a run whose one given text is a file the owned read
// refuses. The file is there and holds a name and a NUL byte, and its path
// holds the name too, so a run that read it, counted it or said its path
// would show it.
func refusedTextAudit(t *testing.T, texts ...string) coreapi.RepoAudit {
	t.Helper()
	w := porttest.NewWorld(t)
	root := w.Seed("repo", map[string]string{"x": "y"})
	w.SeedRepo(root, porttest.Repo{Index: map[string]string{"a.md": "a"},
		Bases: map[[2]string]string{{"origin/main", "HEAD"}: "c0"},
		Heads: map[string]porttest.Rev{"base": {Commit: "c0", Files: map[string]string{"a.md": "a"}}}})
	w.SeedOutside("/data/secretname/shared.txt", "secretname\x00\n")
	w.SeedRefused("/data/secretname/shared.txt", porttest.Refused{Why: "the planted why", Fix: "the planted fix"})
	names, err := format.ParseNames([]byte("secretname\n"))
	if err != nil {
		t.Fatal(err)
	}
	return coreapi.RepoAudit{History: w.Ports().History, FileNames: w.Ports().FileNames, Root: root, Rules: rules(), Names: names,
		Src: fstest.MapFS{"a.md": &fstest.MapFile{Data: []byte("a")}}, Owned: w.Ports().Owned, Texts: texts}
}

// TestARefusedTextIsNamedByItsPlaceWithTheFixedReason: the one row a refused
// text makes names it as given text N, with the fixed reason, and no row or
// finding says its path or the refusal's own words.
func TestARefusedTextIsNamedByItsPlaceWithTheFixedReason(t *testing.T) {
	t.Parallel()
	got := coreapi.CheckRepo(context.Background(), refusedTextAudit(t, "/data/secretname/shared.txt"))
	const want = "given text 1 was not read: it is not owned by this account, or another account can write it"
	var said int
	for _, n := range got.NotChecked {
		if n.Why == want {
			said++
		}
		for _, s := range []string{"/data", "shared.txt", "secretname", "the planted"} {
			if strings.Contains(n.String(), s) {
				t.Errorf("not-checked row %q says %q", n, s)
			}
		}
	}
	if said != 1 {
		t.Errorf("%d rows say %q, want one: %v", said, want, got.NotChecked)
	}
	for _, f := range got.Findings {
		for _, s := range []string{"/data", "shared.txt", "the planted"} {
			if strings.Contains(f.String(), s) {
				t.Errorf("finding %q says %q", f, s)
			}
		}
	}
}

// TestARefusedTextIsNeverReadAndNeverMakesARunClean: against the same run
// with no text, a refused text adds one not-checked row and no finding, and
// no row counts it as read: its name is not found and its NUL byte is not
// counted.
func TestARefusedTextIsNeverReadAndNeverMakesARunClean(t *testing.T) {
	t.Parallel()
	control := coreapi.CheckRepo(context.Background(), refusedTextAudit(t))
	got := coreapi.CheckRepo(context.Background(), refusedTextAudit(t, "/data/secretname/shared.txt"))
	if len(got.NotChecked) != len(control.NotChecked)+1 {
		t.Errorf("not-checked rows %v, want the control's %v and one more", got.NotChecked, control.NotChecked)
	}
	if !slices.Equal(got.Findings, control.Findings) {
		t.Errorf("findings %v, want the control's %v: a refused text was read", got.Findings, control.Findings)
	}
	for _, n := range got.NotChecked {
		if strings.Contains(n.Why, "NUL byte") {
			t.Errorf("row %q counts the refused text as read", n)
		}
	}
}

// TestNoRowOfARunSaysAName: a tracked path that holds a name is said by no
// row, whichever check it comes from, and a text that holds it too is a finding
// at a masked place.
func TestNoRowOfARunSaysAName(t *testing.T) {
	t.Parallel()
	names, err := format.ParseNames([]byte("zorbquux\n"))
	if err != nil {
		t.Fatal(err)
	}
	a := trackedAudit(t, map[string]string{"docs/zorbquux.md": "about zorbquux\n", "docs/ZORBQUUX.md": "x", "docs/clean.md": "ok"}, map[string]string{"docs/zorbquux.lnk": "docs/clean.md"})
	a.Names = names
	got := coreapi.CheckRepo(context.Background(), a)
	var sawName bool
	for _, f := range got.Findings {
		sawName = sawName || f.Key == audit.KeyNames
		for _, s := range []string{f.File, f.Missing, f.String()} {
			if strings.Contains(strings.ToLower(s), "zorbquux") {
				t.Errorf("a finding says the name: %q", s)
			}
		}
	}
	for _, n := range got.NotChecked {
		if strings.Contains(strings.ToLower(n.String()), "zorbquux") {
			t.Errorf("a not-checked row says the name: %q", n)
		}
	}
	if !sawName {
		t.Errorf("no names finding: %v", got.Findings)
	}
}

// TestATypedNilListIsNoList: a nil list pointer in the interface says no list
// was given, as nil does, and not that the list is empty.
func TestATypedNilListIsNoList(t *testing.T) {
	t.Parallel()
	a := trackedAudit(t, map[string]string{"a.md": "a"}, nil)
	var none *format.Names
	a.Names = none
	rows := notChecked(coreapi.CheckRepo(context.Background(), a), audit.KeyNames)
	if len(rows) != 1 || !strings.Contains(rows[0].Why, "no names list was given") {
		t.Errorf("names rows %v, want no names list was given", rows)
	}
}

// TestARefusedListSaysOneOfAClosedSetOfReasons: whatever the error says, the
// row says one reason of four, and holds neither the path nor the pattern of a
// line that does not compile. The error is the one the list's own reader
// ends a bad line with.
func TestARefusedListSaysOneOfAClosedSetOfReasons(t *testing.T) {
	t.Parallel()
	_, perr := format.ParseNames([]byte("fine\n(pattern-secret\n"))
	bad, ok := errors.AsType[*format.BadNames](perr)
	if !ok {
		t.Fatalf("a bad line gave %v", perr)
	}
	for name, err := range map[string]error{
		"a line that does not compile": fmt.Errorf("%w: /data/alice/names.txt: %w", coreapi.ErrNotReady, bad),
		"a refusal":                    fmt.Errorf("%w: %w", coreapi.ErrNotReady, fmt.Errorf("/data/alice/names.txt: %w", port.ErrNotOwned)),
		"a missing file":               fmt.Errorf("open /data/alice/names.txt: %w", fs.ErrNotExist),
		"any other":                    errors.New("read /data/alice/names.txt: boom"),
	} {
		a := trackedAudit(t, map[string]string{"a.md": "a"}, nil)
		a.NamesErr = err
		got := coreapi.CheckRepo(context.Background(), a)
		doc := fmt.Sprint(got.Document())
		for _, s := range []string{"alice", "/data", "pattern-secret", "names.txt"} {
			if strings.Contains(doc, s) {
				t.Errorf("%s: the document holds %q: %s", name, s, doc)
			}
		}
		if rows := notChecked(got, audit.KeyNames); len(rows) != 1 {
			t.Errorf("%s: names rows %v", name, rows)
		}
	}
}

// TestASkippedFileIsCountedAndNeverSilentlyClean: a tracked file with a NUL
// byte, and one the work tree cannot read, are each one row with a count.
func TestASkippedFileIsCountedAndNeverSilentlyClean(t *testing.T) {
	t.Parallel()
	names, err := format.ParseNames([]byte("zorbquux\n"))
	if err != nil {
		t.Fatal(err)
	}
	a := trackedAudit(t, map[string]string{"a.bin": "zorbquux\x00", "b.md": "ok", "gone.md": "x"}, nil)
	delete(a.Src.(fstest.MapFS), "gone.md")
	a.Names = names
	got := coreapi.CheckRepo(context.Background(), a)
	var nul, unread bool
	for _, n := range notChecked(got, audit.KeyNames) {
		nul = nul || strings.Contains(n.Why, "1 files hold a NUL byte")
		unread = unread || strings.Contains(n.Why, "1 tracked files could not be read")
	}
	if !nul || !unread {
		t.Errorf("rows %v, want a count of the binary and of the unread", got.NotChecked)
	}
}

// TestAnOutsidePathThatHoldsANameIsSaidByNoRow: a text whose own path holds the
// name, found or unread, is told by no finding and no row, and an unreadable
// one gives its row no words of the port's error.
func TestAnOutsidePathThatHoldsANameIsSaidByNoRow(t *testing.T) {
	t.Parallel()
	w := porttest.NewWorld(t)
	root := w.Seed("repo", map[string]string{"x": "y"})
	w.SeedRepo(root, porttest.Repo{Index: map[string]string{"a.md": "a"},
		Bases: map[[2]string]string{{"origin/main", "HEAD"}: "c0"},
		Heads: map[string]porttest.Rev{"base": {Commit: "c0", Files: map[string]string{"a.md": "a"}}}})
	w.SeedOutside("/data/zorbquux/note.txt", "about zorbquux\n")
	names, err := format.ParseNames([]byte("zorbquux\n"))
	if err != nil {
		t.Fatal(err)
	}
	a := coreapi.RepoAudit{History: w.Ports().History, FileNames: w.Ports().FileNames, Root: root, Rules: rules(), Names: names,
		Src: fstest.MapFS{"a.md": &fstest.MapFile{Data: []byte("a")}}, Owned: w.Ports().Owned,
		Texts: []string{"/data/zorbquux/note.txt", "/data/zorbquux/missing.txt", "zorbquux-relative.txt"}}
	got := coreapi.CheckRepo(context.Background(), a)
	if doc := strings.ToLower(fmt.Sprint(got.Document())); strings.Contains(doc, "zorbquux") {
		t.Errorf("the document says the name: %s", doc)
	}
	if strings.Contains(fmt.Sprint(got.Document()), "/data/") {
		t.Errorf("the document says a path: %v", got.Document())
	}
	if len(got.Findings) == 0 || len(notChecked(got, audit.KeyNames)) != 2 {
		t.Errorf("findings %v, rows %v", got.Findings, got.NotChecked)
	}
}
