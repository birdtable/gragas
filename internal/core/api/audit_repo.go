package coreapi

import (
	"context"
	"fmt"
	"io/fs"
	"strings"

	"github.com/birdtable/gragas/internal/core/audit"
	"github.com/birdtable/gragas/internal/core/format"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// maxRead is the most of one file the audit reads: a larger one is left
// unread and the run says so, never a part of it.
const maxRead = 8 << 20

// DefaultBase is the revision the counts and the range are taken against when
// the run names none.
const DefaultBase = "origin/main"

// RepoAudit is what the repository checks of gragas check read: the tracked
// files and the history, and the private list, which it is handed
// already read. Each check is a function of the audit package over what is
// read here; this reads it.
type RepoAudit struct {
	// NoGit says git cannot be set up, which leaves every check of the three
	// unjudged and History unset.
	NoGit   bool
	History port.History
	// Root is the repository, opened for History.
	Root model.Root
	// Src reads the work tree by slash-separated paths relative to the root.
	Src fs.ReadFileFS
	// Names is the private list as readBookNames read it, nil for none. A list
	// the owned read refused is NamesErr, and the names check says it was not
	// made and why; the run never reads the list itself.
	Names    NameList
	NamesErr error
	// Texts are files outside the repository that the names check reads too,
	// each an absolute, clean path, read through Owned.ReadOwned and kept to
	// maxRead. A text that cannot be read, or that another account owns or
	// can write, is not checked, and says why.
	Texts []string
	Owned port.Owned
	// FileNames answers each tracked path's key, name by name: it is never nil in a run that reads the
	// tracked files.
	FileNames port.FileNames
	// Base is the revision whose merge base with HEAD the counts compare
	// against; "" is DefaultBase. Range says the commit messages since that
	// merge base are read for names too.
	Base  string
	Range bool
	// Rules say which rule each check reports under; Counting is what the
	// count check reads of each tool (the readers table's, Countings).
	Rules    map[string]string
	Counting []Counting
}

// RepoFindings is what the three checks found, twice: in the shapes of the
// config checks' findings, and in the project check's (Gate and Unjudged),
// which check below a pin lists. Both are masked alike.
type RepoFindings struct {
	Findings   []ConfigFinding
	NotChecked []ConfigNotChecked
	// Gate is each finding as the project check lists it, in the order of
	// model.ProjectKinds; Unjudged is each row that says a check was not made.
	Gate     []model.GateFinding
	Unjudged []model.NotChecked
}

// repoRun is what the checks have found so far, under the rules they report
// under, and which tracked paths there are.
type repoRun struct {
	rules    audit.RepoRules
	tracked  map[string]bool
	Findings []audit.Finding
	Rows     []repoRow
}

// NameList is the private list as the run asks of it: whether it has a name,
// and whether a text holds one. The list readBookNames returns is one.
type NameList interface {
	Find(text string) string
	Len() int
}

// maskedText stands where a name was in a row that says a path or a reason.
const maskedText = "<name>"

// maskName is s with each name of the list in it hidden, so that no row of a
// run, whatever check it comes from, says a name: the path of a tracked file
// can hold one. A name found by its hash has no text to cut, so the whole of s
// goes. With no list it is s.
func maskName(l NameList, s string) string {
	for range 64 {
		if l == nil || s == "" {
			return s
		}
		m := l.Find(s)
		if m == "" {
			return s
		}
		if !strings.Contains(s, m) {
			return maskedText
		}
		s = strings.ReplaceAll(s, m, maskedText)
	}
	return maskedText
}

// matcher is the list as the names check asks of it.
type matcher struct{ n NameList }

func (m matcher) Find(text string) bool { return m.n.Find(text) != "" }

// CheckRepo runs the names, tracked-files and count checks.
func CheckRepo(ctx context.Context, a RepoAudit) RepoFindings {
	if n, ok := a.Names.(*format.Names); ok && n == nil {
		a.Names = nil // a nil list pointer is no list, not an empty one
	}
	return checkRepo(ctx, a).masked(a.Names)
}

func checkRepo(ctx context.Context, a RepoAudit) repoRun {
	out := repoRun{rules: audit.RepoRules(a.Rules), tracked: map[string]bool{}}
	if a.NoGit {
		out.cannotAll(model.CauseHistory, "git is not set up here, so the tracked files and the history cannot be read")
		return out
	}
	files, err := a.History.Files(ctx, a.Root, "")
	if err != nil {
		out.cannotAll(model.CauseHistory, "the tracked files could not be read: "+err.Error())
		return out
	}
	if len(files) == 0 {
		for _, k := range []string{audit.KeyNames, audit.KeyTracked, audit.KeyCounts} {
			out.row(k, "git answered an empty list of tracked files, so there is nothing to judge")
		}
		return out
	}
	tracked := make([]audit.Tracked, 0, len(files))
	for _, f := range files {
		if !f.Dir {
			t := audit.Tracked{Path: f.Path, Link: f.Link}
			t.Key, t.Refused = portableKey(a.FileNames, f.Path)
			tracked = append(tracked, t)
			out.tracked[f.Path] = true
		}
	}
	found, unchecked := audit.CheckTracked(out.rules, tracked)
	out.Findings = append(out.Findings, found...)
	out.unfolded(unchecked)

	takes, skipped, unread := a.readTracked(tracked)
	for _, p := range skipped {
		out.part(audit.KeyNames, p+" is larger than "+fmt.Sprint(maxRead>>20)+" MiB and was not read")
	}
	more, refused := a.readTexts()
	takes = append(takes, more...)
	if n := binaryCount(takes); n > 0 {
		// Not checked, and never a halt: a tracked image would make every
		// repository not ready.
		out.part(audit.KeyNames, fmt.Sprint(n)+" files hold a NUL byte and were not read for names")
	}
	if unread > 0 {
		out.part(audit.KeyNames, fmt.Sprint(unread)+" tracked files could not be read from the work tree and were not read for names")
	}
	base, baseErr := a.mergeBase(ctx)
	out.names(ctx, a, namesIn{tracked: tracked, takes: takes, refused: refused}, base, baseErr)
	out.counts(ctx, a, takes, base, baseErr)
	return out
}

// portableKey is the key the FileNames port gives a tracked path, each of its names asked through
// Portable and joined by "/", a name the port refuses standing for itself so the names beside it are still
// compared, and why for the first such name, "" when none is. The core keeps no table of what a disk does
// with a name.
func portableKey(file port.FileNames, p string) (key, why string) {
	names := strings.Split(p, "/")
	keys := make([]string, len(names))
	for i, name := range names {
		k, err := file.Portable(name)
		if err != nil {
			if why == "" {
				why = err.Error()
			}
			k = "\x00" + name
		}
		keys[i] = k
	}
	return strings.Join(keys, "/"), why
}

// Document is the repository object of check's --json: the findings and the
// checks that could not be made, each a list and never null.
func (r RepoFindings) Document() map[string]any {
	return map[string]any{"findings": append([]ConfigFinding{}, r.Findings...), "not_checked": append([]ConfigNotChecked{}, r.NotChecked...)}
}

// UnopenedRepo says, a row for each of the three checks, that the repository
// could not be opened and so none of them ran. The reason is masked as
// CheckRepo masks its rows: the repository's path can hold a login, and a
// login may be on the list. A check a rule binds halts the run, as in
// CheckRepo.
func UnopenedRepo(l NameList, rules map[string]string, err error) RepoFindings {
	if n, ok := l.(*format.Names); ok && n == nil {
		l = nil
	}
	r := repoRun{rules: audit.RepoRules(rules)}
	r.cannotAll(model.CauseHistory, "the repository could not be opened: "+err.Error())
	return r.masked(l)
}
