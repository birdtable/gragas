package coreapi

import (
	"context"
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/core/audit"
	"github.com/birdtable/gragas/internal/model"
)

// The checks a repository run makes from what it read: names and counts.

// What the names check and the counts say when no rule binds them.
const (
	noNamesRule = "no rule of the rulebook binds the names key, so the names check did not run"
	noCountRule = "no rule of the rulebook binds a count key, so the counts did not run"
)

// namesIn is what the names check reads: the tracked paths, the takes of the
// texts read (tracked and given), and why each given text that was not read was not.
type namesIn struct {
	tracked []audit.Tracked
	takes   []take
	refused []string
}

func (r *repoRun) names(ctx context.Context, a RepoAudit, n namesIn, base string, baseErr error) {
	in := audit.NamesInput{Rules: audit.RepoRules(a.Rules)}
	switch {
	case !audit.RepoRules(a.Rules).Has(audit.KeyNames):
		r.row(audit.KeyNames, noNamesRule)
		return
	case a.NamesErr != nil:
		r.cannot(audit.KeyNames, model.CauseNamesList, namesNotRead(a.NamesErr))
		return
	case a.Names != nil && a.Names.Len() == 0:
		r.row(audit.KeyNames, "the names list is empty, so the names check did not run")
		return
	case a.Names != nil:
		in.List = matcher{a.Names}
	}
	// A given text not read, and a range whose messages were not read, are
	// each an input the person named: the check they asked for was not made,
	// and ending done would pass it unread, so each halts the run when names
	// binds. A tracked file not read is a
	// part: the rest of the tree was judged.
	for _, why := range n.refused {
		r.cannot(audit.KeyNames, model.CauseText, why)
	}
	for _, t := range n.tracked {
		in.Paths = append(in.Paths, t.Path)
	}
	if a.Range {
		msgs, err := a.messages(ctx, base, baseErr)
		if err != nil {
			r.cannot(audit.KeyNames, baseOr(baseErr), "the commit messages of the range could not be read: "+err.Error())
		}
		in.Messages = msgs
	}
	found, nc := audit.CheckNames(in)
	if nc == nil {
		for _, t := range n.takes {
			found = append(found, t.Hits...)
		}
		slices.SortStableFunc(found, byPlace)
	}
	r.Findings = append(r.Findings, found...)
	if nc != nil {
		r.Rows = append(r.Rows, repoRow{NotChecked: *nc}) // no list named: a row, as the project chose
	}
}

// baseOr is the cause of a read of the range that failed: the merge base when
// it was not found, since nothing of the range is read without it, and the
// history otherwise.
func baseOr(baseErr error) model.GateCause {
	if baseErr != nil {
		return model.CauseBase
	}
	return model.CauseHistory
}

// namesNotRead is why the list was not read, without the file's path: the
// path may hold a login, and a login may be on the list. The reason is
// namesReason's closed set.
func namesNotRead(err error) string {
	return namesReason(err) + ", so the names check did not run"
}

func (a RepoAudit) messages(ctx context.Context, base string, baseErr error) ([]audit.Message, error) {
	if baseErr != nil {
		return nil, baseErr
	}
	head, err := a.History.Base(ctx, a.Root, "HEAD", "HEAD")
	if err != nil {
		return nil, err
	}
	ms, err := a.History.Messages(ctx, a.Root, base, head)
	if err != nil {
		return nil, err
	}
	out := make([]audit.Message, len(ms))
	for i, m := range ms {
		out[i] = audit.Message{Commit: m.Commit, Text: m.Text}
	}
	return out, nil
}

// counted is the files whose lines the count check reads: Go files and the
// tools' configs, in the tracked list.
func (a RepoAudit) counted(p string) bool {
	return strings.HasSuffix(p, ".go") || slices.ContainsFunc(a.Counting, func(c Counting) bool {
		return slices.Contains(c.Configs, path.Base(p))
	})
}

// counts reads the three counts of the work tree against the merge base.
// "Now" is the work tree: each file's text as its take read it through Src,
// staged or not. The base side reads a blob for each counted file of the
// merge base but one that is unchanged, and a file is unchanged only when
// its object id at the merge base equals the id git gives its work-tree
// bytes, hashed with no filter in one run over the counted paths
// (History.WorkObjects); its lines then stand on both sides.
// Anything else is read: an entry with no id, a path given no id now, a
// hash that fails, and a file git would filter, whose bytes on disk are not
// what git stored. One git read for each of a thousand files took nine
// minutes; the ids leave the reads to the files that changed. gragas makes
// no object id of its own.
//
// The take's read and git's hash are two reads of the work tree, made one
// after the other. A file edited between them that is back at its base
// bytes when git hashes it is taken as unchanged, with the lines the take
// read: a gap only a tree edited during the run opens, and a landing tree
// nobody edits closes it.
func (r *repoRun) counts(ctx context.Context, a RepoAudit, takes []take, base string, baseErr error) {
	if rules := audit.RepoRules(a.Rules); !rules.Has(audit.KeyDebt) && !rules.Has(audit.KeyDirective) {
		r.row(audit.KeyCounts, noCountRule)
		return
	}
	if baseErr != nil {
		r.cannot(audit.KeyCounts, model.CauseBase, baseErr.Error())
		return
	}
	files, err := a.History.Files(ctx, a.Root, base)
	if err != nil {
		r.cannot(audit.KeyCounts, model.CauseHistory, "the files at the merge base could not be read: "+err.Error())
		return
	}
	object := map[string]string{}
	for _, f := range files {
		if !f.Dir && !f.Link && a.counted(f.Path) {
			object[f.Path] = f.Object
		}
	}
	var head []audit.Counted
	for _, t := range takes {
		if a.counted(t.Path) {
			head = append(head, t.Counted...)
		}
	}
	was := unchanged(ctx, a, takes, object)
	count := audit.NewCounter(countings(a.Counting))
	for p := range object {
		b, err := a.History.Blob(ctx, a.Root, base, p)
		if err != nil {
			r.cannot(audit.KeyCounts, model.CauseHistory, p+" could not be read at the merge base: "+err.Error())
			return
		}
		if len(b) > maxRead {
			r.cannot(audit.KeyCounts, model.CauseHistory, p+" is larger than "+fmt.Sprint(maxRead>>20)+" MiB at the merge base and was not read")
			return
		}
		was = append(was, count(audit.Doc{Path: p, Text: string(b)})...)
	}
	r.Findings = append(r.Findings, audit.CheckCounts(audit.RepoRules(a.Rules), was, head)...)
}

// unchanged is the counted lines of each take whose work-tree id is the id
// object holds for its path at the merge base, a non-empty one, and takes
// each such path out of object, so that what object keeps is read. object
// holds the counted files of the merge base, so a take of no counted file is
// never asked for.
func unchanged(ctx context.Context, a RepoAudit, takes []take, object map[string]string) []audit.Counted {
	var asked []take
	for _, t := range takes {
		if _, ok := object[t.Path]; ok {
			asked = append(asked, t)
		}
	}
	var was []audit.Counted
	for i, id := range workObjects(ctx, a, asked) {
		if p := asked[i].Path; id != "" && id == object[p] {
			was = append(was, asked[i].Counted...)
			delete(object, p)
		}
	}
	return was
}

// workObjects is git's id of each of takes' work-tree bytes, in their order,
// from one History.WorkObjects. None is asked for no take. A hash that fails,
// or an answer that is not one id a take, is none: each file is then read at
// the merge base, which costs reads and lets no file through.
func workObjects(ctx context.Context, a RepoAudit, takes []take) []string {
	if len(takes) == 0 {
		return nil
	}
	paths := make([]string, len(takes))
	for i, t := range takes {
		paths[i] = t.Path
	}
	ids, err := a.History.WorkObjects(ctx, a.Root, paths)
	if err != nil || len(ids) != len(paths) {
		return nil
	}
	return ids
}

// byPlace orders findings as the audit package does: by file, line, rule and
// what is missing. The names check sorts its own findings that way, and the
// takes' are joined to them here.
func byPlace(x, y audit.Finding) int {
	switch {
	case x.File != y.File:
		return strings.Compare(x.File, y.File)
	case x.Line != y.Line:
		return x.Line - y.Line
	case x.Rule != y.Rule:
		return strings.Compare(x.Rule, y.Rule)
	}
	return strings.Compare(x.Missing, y.Missing)
}
