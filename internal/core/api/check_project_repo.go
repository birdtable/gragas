package coreapi

import (
	"context"
	"io/fs"
	"maps"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/core/audit"
	"github.com/birdtable/gragas/internal/core/project"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// The three repository checks in the project check: what
// checkProject hands CheckRepo, and the rows about the names list.

// repoChecks runs the names, tracked-files and counts checks over the project
// l: its tracked files and its history, read through the History the ask
// opens, and the work tree through src, the Tree port's confined view of the
// project's root. The list is the one the book's load read (rep), never read
// a second time; the rules are the pinned parts' (repoRules); the merge base
// is HEAD's with the ask's base ref. The commit messages are not read, and no
// text outside the repository is, so the check is handed no Owned port. file
// is the session's FileNames port, which gives each tracked path its key for
// the tracked-files check and is asked on the first tracked path.
//
// When no rule of the pinned parts binds any of the three, nothing is opened
// and nothing is read: the names check and the counts say so, as CheckRepo
// says it of a repository it read. A build with no git (no opener) leaves each
// of the three not checked, and so does a History that cannot be opened, with
// its reason masked: either ends the run not ready when a rule of the pinned
// parts binds the check.
//
// A key of the program that a rule of the pinned parts binds and none of the
// three judges is a row of its own after theirs (unjudgedKeys).
func repoChecks(ctx context.Context, file port.FileNames, l project.Loaded, rep BookReport, src fs.FS, ask ProjectAsk) RepoFindings {
	rules := repoRules(rep.Book, l.Pin.Parts)
	out := repoJudged(ctx, ask.OpenHistory, RepoAudit{
		Root: l.Root, Src: readFS{src}, FileNames: file, Names: rep.names, Base: ask.BaseRef,
		Rules: rules, Counting: Countings(ask.Readers.Rows),
	})
	out.Unjudged = append(out.Unjudged, unjudgedKeys(rules)...)
	return out
}

// repoJudged is what the three checks found in the run a describes, and their
// rows: a's history is the one open opens, and only when a rule binds a check.
func repoJudged(ctx context.Context, open func() (port.History, error), a RepoAudit) RepoFindings {
	if !slices.ContainsFunc(repoKeys(), func(k string) bool { return a.Rules[k] != "" }) {
		r := repoRun{}
		r.row(audit.KeyNames, noNamesRule)
		r.row(audit.KeyCounts, noCountRule)
		return r.masked(nil)
	}
	if open == nil {
		return CheckRepo(ctx, RepoAudit{NoGit: true, Names: a.Names, Rules: a.Rules})
	}
	h, err := open()
	if err != nil {
		return UnopenedRepo(a.Names, a.Rules, err)
	}
	a.History = h
	return CheckRepo(ctx, a)
}

// unjudgedKeys is a row for each key of the program that a rule of the pinned
// parts binds and none of the three checks judges, replace among them: the
// row names the key and the rule, so the run says the rule was not checked
// and never passes it in silence. It leaves the ending as it is, since the
// project cannot cure it.
func unjudgedKeys(rules map[string]string) []model.NotChecked {
	var out []model.NotChecked
	for _, k := range slices.Sorted(maps.Keys(rules)) {
		if !slices.Contains(repoKeys(), k) {
			why := rules[k] + " binds the " + k + " key, which no check below a pin judges"
			out = append(out, model.NotChecked{Msg: audit.Unjudged(k, why).String()})
		}
	}
	return out
}

// repoKeys is the keys the three checks judge under.
func repoKeys() []string {
	return []string{audit.KeyNames, audit.KeyLink, audit.KeyFold, audit.KeyDebt, audit.KeyDirective}
}

// repoRules is the rule each computed key of the program is checked under: the
// rung 0 bindings of the active and trial rules of the parts that name the
// program and a key it computes, which projectBindings leaves to these checks.
func repoRules(b model.Book, parts []string) map[string]string {
	var out []RepoBinding
	for _, r := range b.Rules {
		if r.Status != model.Active && r.Status != model.Trial {
			continue
		}
		id := string(r.ID)
		if part, _, _ := strings.Cut(id, "."); !slices.Contains(parts, part) {
			continue
		}
		for _, bd := range r.Bindings {
			if bd.Rung == 0 && bd.Tool == audit.Program && audit.Computed(bd.Key) {
				out = append(out, RepoBinding{Rule: id, Tool: bd.Tool, Key: bd.Key})
			}
		}
	}
	return RepoRulesOf(out)
}

// readFS reads a whole file of an fs.FS by name, as the repository checks ask.
type readFS struct{ fs.FS }

func (r readFS) ReadFile(name string) ([]byte, error) { return fs.ReadFile(r.FS, name) }

// namesRows say when the book's names check did not run: no list named, or the
// load's notice that says why it did not.
func namesRows(namesFile string, notices []model.Notice) []model.NotChecked {
	if namesFile == "" {
		return []model.NotChecked{{Msg: "names: no list named, so the names check did not run"}}
	}
	var out []model.NotChecked
	for _, n := range notices {
		if strings.HasPrefix(n.Msg, "the names check did not run") {
			out = append(out, model.NotChecked{Msg: "names: " + n.Msg})
		}
	}
	return out
}
