package internal

// The exceptions gragas makes to its checks, held to suppressions.txt: a
// lint suppression (gates.exemptions.1, .2, go.errors.1) and an error read
// by its text (go.errors.8). The list only shrinks.

import (
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// nolintHead is how golangci-lint v2.15.0's nolint filter knows a directive
// (pkg/result/processors/nolint_filter.go), past the comment's leading
// slashes and spaces: nolint, then a space, a colon or the end. A nolint
// with no colon, or nolint:all, silences every linter.
var nolintHead = regexp.MustCompile(`^nolint( |:|$)`)

// directive is the comment text read as that filter reads it: its linters,
// in lower case and trimmed, or "all"; its reason, the text after a second
// //; and whether it is a nolint at all. A name with a dash, an underscore or
// a capital, or a space after the colon, makes one too (gragas#1750). A
// comment of prose that opens with the word and a space is one as well, so
// no comment here opens so.
func directive(text string) (linters []string, reason string, ok bool) {
	t := strings.TrimLeft(text, "/ ")
	if !nolintHead.MatchString(t) {
		return nil, "", false
	}
	directive, reason, _ := strings.Cut(t, "//")
	reason = strings.TrimSpace(reason)
	if strings.HasPrefix(t, "nolint:all") || !strings.HasPrefix(t, "nolint:") {
		return []string{"all"}, reason, true
	}
	for item := range strings.SplitSeq(strings.TrimPrefix(directive, "nolint:"), ",") {
		linters = append(linters, strings.ToLower(strings.TrimSpace(item)))
	}
	return linters, reason, true
}

// banLinters are the linters the bans are written for: no nolint names one,
// nor all, which holds them (gates.exemptions.1, gragas#1750). A ban that
// does not fit a call site is changed in the lint source, where review reads
// it, never silenced at the site.
func banLinters() []string { return []string{"depguard", "forbidigo", "gocritic", "nolintlint", "all"} }

// serialReason is what the reason of a suppression of the parallel-test
// linters begins with (go.cycle.2, tests.runners.4): nolintlint can require an
// explanation but not its text.
const serialReason = "serial:"

// serialLinter is whether the comma-separated linters hold paralleltest or
// tparallel.
func serialLinter(linters string) bool {
	return slices.ContainsFunc(strings.Split(linters, ","), func(l string) bool { return l == "paralleltest" || l == "tparallel" })
}

// suppressions is every nolint comment of all, as "nolint <file> <linters>"
// lines, and every one with no reason or no linter named.
func suppressions(all []source) (lines, bad []string) {
	for _, s := range all {
		for _, cg := range s.file.Comments {
			for _, c := range cg.List {
				linters, reason, ok := directive(c.Text)
				if !ok {
					continue
				}
				names := strings.Join(linters, ",")
				if names == "all" || reason == "" {
					bad = append(bad, s.path+": "+c.Text+": a suppression names its linter and gives its reason")
				}
				for _, l := range linters {
					if slices.Contains(banLinters(), l) {
						bad = append(bad, s.path+": "+c.Text+": a suppression of "+l+" silences a ban; fix the call, or change the ban in the lint source (gragas#1750)")
					}
				}
				if serialLinter(names) && !strings.HasPrefix(reason, serialReason) {
					bad = append(bad, s.path+": "+c.Text+": a suppression of paralleltest or tparallel begins its reason with "+serialReason+" and names the state the test needs (tests.runners.4)")
				}
				lines = append(lines, "nolint "+s.path+" "+names)
			}
		}
	}
	return lines, bad
}

// textMatchers are the calls that read text the way a caller would match
// an error's message.
func textMatchers() []string {
	return []string{"HasPrefix", "HasSuffix", "Contains", "ContainsAny", "TrimPrefix", "TrimSuffix", "Cut", "CutPrefix", "CutSuffix",
		"Index", "LastIndex", "EqualFold", "Split", "SplitN", "Fields", "Replace", "ReplaceAll", "Count", "MatchString", "FindString", "FindStringSubmatch"}
}

// errorText is go.errors.8 in every non-test file: an error's text, from an
// Error() call or a variable holding one, is never compared, matched, cut or
// split; an error is compared with errors.Is or errors.As. Text built from
// an error for a person to read is fine.
func errorText(all []source) []string {
	var out []string
	for _, s := range all {
		if s.test {
			continue
		}
		found := false
		ast.Inspect(s.file, func(n ast.Node) bool {
			fn, ok := n.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				return true
			}
			if readsErrorText(fn.Body) {
				found = true
			}
			return false
		})
		if found {
			out = append(out, "error-text "+s.path)
		}
	}
	return out
}

// readsErrorText is whether body matches an error's text.
func readsErrorText(body *ast.BlockStmt) bool {
	holds := map[string]bool{}
	carries := func(e ast.Expr) bool {
		hit := false
		ast.Inspect(e, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.CallExpr:
				if sel, ok := x.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Error" && len(x.Args) == 0 {
					hit = true
				}
			case *ast.Ident:
				if holds[x.Name] {
					hit = true
				}
			}
			return !hit
		})
		return hit
	}
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.AssignStmt:
			for i, r := range x.Rhs {
				if id, ok := x.Lhs[min(i, len(x.Lhs)-1)].(*ast.Ident); ok && carries(r) && !concat(r) {
					holds[id.Name] = true
				}
			}
		case *ast.BinaryExpr:
			if (x.Op == token.EQL || x.Op == token.NEQ) && (carries(x.X) || carries(x.Y)) {
				found = true
			}
		case *ast.CallExpr:
			if sel, ok := x.Fun.(*ast.SelectorExpr); ok && slices.Contains(textMatchers(), sel.Sel.Name) && slices.ContainsFunc(x.Args, carries) {
				found = true
			}
		}
		return true
	})
	return found
}

// concat is whether e builds text by joining: a message for a person.
func concat(e ast.Expr) bool {
	b, ok := e.(*ast.BinaryExpr)
	return ok && b.Op == token.ADD
}

// suppressionsFile is the list of exceptions, at the root.
const suppressionsFile = "suppressions.txt"

// listed reads suppressions.txt as its lines, and the lines with no reason.
func listed(t *testing.T) (lines, bad []string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, suppressionsFile))
	if err != nil {
		t.Fatal(err)
	}
	lines, bad = parseListed(string(data))
	return lines, bad
}

// parseListed is the entries of a suppressions list, each with its spaces
// made single, and the entries with no reason.
func parseListed(data string) (lines, bad []string) {
	for line := range strings.Lines(data) {
		entry, reason, _ := strings.Cut(line, "#")
		entry = strings.Join(strings.Fields(entry), " ")
		if entry == "" {
			continue
		}
		if strings.TrimSpace(reason) == "" {
			bad = append(bad, entry+": a listed exception gives its reason")
		}
		lines = append(lines, entry)
	}
	return lines, bad
}

// admitted is the entries suppressions.txt may hold, once each, though the
// list at the merge base does not: the one place the list is let grow, by a
// ruling and for a time. It admits none: the one it held, the nolint of
// internal/cli/root/commands.go, went with its line and its suppression when
// the kit's name for a signal's stop came in (gragas#1962). An entry here
// that the list no longer holds is red (gragas#1963).
func admitted() []string {
	return nil
}

// grown is each entry of list that base, the list at the merge base, does
// not hold as often: the list only shrinks (gates.exemptions.2,
// gragas#1750). A file moved or renamed with its suppression is an entry
// added, and is decided again. An entry of admit is let by once, as written:
// a second one, the same linter in another file and another linter in the same
// file are each an entry added. An entry of admit the list no longer holds is
// a problem of its own, so an admission never outlives its line.
func grown(list, base string, admit []string) []string {
	now, _ := parseListed(list)
	then, _ := parseListed(base)
	extra := difference(now, then)
	var out []string
	for _, a := range admit {
		if !slices.Contains(now, a) {
			out = append(out, a+": is admitted in admitted() of internal/exceptions_test.go, and "+suppressionsFile+" no longer holds it; delete the admission")
			continue
		}
		if i := slices.Index(extra, a); i >= 0 && !slices.Contains(then, a) {
			extra = slices.Delete(extra, i, i+1)
		}
	}
	for _, x := range extra {
		out = append(out, x+": is in "+suppressionsFile+" and not in the list at the merge base; the list only shrinks, so fix the code instead")
	}
	return out
}

// difference is what want holds more often than have, one line per extra.
func difference(want, have []string) []string {
	left := slices.Clone(have)
	var out []string
	for _, w := range want {
		if i := slices.Index(left, w); i >= 0 {
			left = slices.Delete(left, i, i+1)
			continue
		}
		out = append(out, w)
	}
	return out
}

// exceptions is the tree's exceptions held to the list: an unlisted one, a
// stale line, a suppression or a line with no reason.
func exceptions(found, list, bad []string) []string {
	out := slices.Clone(bad)
	for _, x := range difference(found, list) {
		out = append(out, x+": not in suppressions.txt; fix it, or list it with its reason")
	}
	for _, x := range difference(list, found) {
		out = append(out, x+": listed in suppressions.txt, and the tree no longer needs it; delete the line")
	}
	slices.Sort(out)
	return out
}

// exceptionChecks is the exceptions of the tree against suppressions.txt.
func exceptionChecks(t *testing.T, all []source) []string {
	t.Helper()
	found, bad := suppressions(all)
	found = append(found, errorText(all)...)
	list, badList := listed(t)
	return exceptions(found, list, append(bad, badList...))
}

// TestTheTreesSuppressionsAreTheListsOwn is the suppressions part of
// TestTheTreeKeepsTheTable on its own, so the push runs it (gragas#2130): a
// suppression mark, or an error read by its text, that the suppressions list does
// not hold is red at the push and not only on the merged tree. Risk: a push
// that adds a bare suppression of gosec to a test file passes the hook and is
// caught at landing.
func TestTheTreesSuppressionsAreTheListsOwn(t *testing.T) {
	t.Parallel()
	_, w := tree(t)
	for _, f := range exceptionChecks(t, sources(t, w)) {
		t.Error(f)
	}
}

func TestAnUnlistedOrUnexplainedExceptionFails(t *testing.T) {
	t.Parallel()
	src := parse(t, "internal/core/rulebook", "book.go", `package rulebook

import (
	"strings"
	"os"
)

func a(err error) bool { return strings.TrimPrefix(err.Error(), ErrSyntax.Error()+": ") != "" }

func b(err error) bool {
	msg := strings.TrimSpace(err.Error())
	return strings.HasPrefix(msg, "unknown")
}

func c(err error) bool { return err.Error() == "busy" }

func d(err error) string { return "read: " + err.Error() }

func e() { _ = os.Remove("x") //nolint:errcheck
}

func f() { _ = os.Remove("y") //nolint
}

func g() { _ = os.Remove("z") //nolint:paralleltest // it sets the environment
}

func h() { _ = os.Remove("w") //nolint:tparallel,paralleltest // serial: it sets the environment
}
`)
	if !readsErrorText(funcBody(t, src, "a")) || !readsErrorText(funcBody(t, src, "b")) || !readsErrorText(funcBody(t, src, "c")) {
		t.Error("an error read by its text was not seen")
	}
	if readsErrorText(funcBody(t, src, "d")) {
		t.Error("a message built for a person was refused")
	}
	found, bad := suppressions([]source{src})
	found = append(found, errorText([]source{src})...)
	got := exceptions(found, []string{"nolint internal/old.go errcheck"}, bad)
	for _, want := range []string{
		"//nolint:errcheck: a suppression names its linter and gives its reason",
		"//nolint: a suppression names its linter and gives its reason",
		"//nolint:paralleltest // it sets the environment: a suppression of paralleltest or tparallel begins its reason with serial: and names the state the test needs (tests.runners.4)",
		"nolint internal/core/rulebook/book.go errcheck: not in suppressions.txt",
		"error-text internal/core/rulebook/book.go: not in suppressions.txt",
		"nolint internal/old.go errcheck: listed in suppressions.txt, and the tree no longer needs it",
	} {
		fires(t, got, want)
	}
	for _, b := range bad {
		if strings.Contains(b, "tparallel,paralleltest") {
			t.Errorf("a suppression that gives its serial reason was refused: %s", b)
		}
	}
}

func funcBody(t *testing.T, s source, name string) *ast.BlockStmt {
	t.Helper()
	for _, d := range s.file.Decls {
		if fn, ok := d.(*ast.FuncDecl); ok && fn.Name.Name == name {
			return fn.Body
		}
	}
	t.Fatalf("no func %s", name)
	return nil
}

// TestTheSuppressionsOnlyShrink holds suppressions.txt to the list at its
// merge bases with origin/main and origin/release-1 (gates.exemptions.2,
// gragas#1750, gragas#2317: release-1 is a long-lived branch whose reviewed
// suppressions are not on main, as for toolPlacesFile): a line neither base
// holds, or a file's line moved to another file, is red. With no merge base
// it is red and says why. An entry admitted() names is let by once
// (gragas#1963): twice, the same linter in another file and another linter in
// the same file are red, and so is an admission whose line is gone.
func TestTheSuppressionsOnlyShrink(t *testing.T) {
	t.Parallel()
	list, err := os.ReadFile(filepath.Join(root, suppressionsFile))
	if err != nil {
		t.Fatal(err)
	}
	base, why, _ := osListBase(t, root, suppressionsFile, "refs/remotes/origin/release-1")
	if why != "" {
		t.Fatal(why)
	}
	for _, p := range grown(string(list), base, admitted()) {
		t.Error(p)
	}
	const a, b = "nolint x/a.go gosec # a reason\n", "nolint x/b.go errcheck # a reason\n"
	// k is a planted admitted entry, as a line of the list, and admit the admission of it.
	const k = "nolint x/k.go errorlint # a reason\n"
	admit := []string{"nolint x/k.go errorlint"}
	for _, tc := range []struct {
		name, list, base string
		admit            []string
		want             []string
	}{
		{"the list as the base", a + b, a + b, nil, nil},
		{"a line gone", a, a + b, nil, nil},
		{"spacing and reasons", "nolint   x/a.go gosec # another reason\n", a, nil, nil},
		{"a line added", a + b, a, nil, []string{"nolint x/b.go errcheck: is in suppressions.txt and not in the list at the merge base"}},
		{"a line repeated", a + a, a, nil, []string{"nolint x/a.go gosec: is in"}},
		{"a line moved to another file", "nolint x/c.go gosec # a reason\n", a, nil, []string{"nolint x/c.go gosec: is in"}},
		{"the admitted entry once, absent from the base", a + k, a, admit, nil},
		{"the admitted entry, with no admission", a + k, a, nil, []string{"nolint x/k.go errorlint: is in"}},
		{"the admitted entry twice", a + k + k, a, admit, []string{"nolint x/k.go errorlint: is in"}},
		{"the admitted entry twice, once in the base", a + k + k, a + k, admit, []string{"nolint x/k.go errorlint: is in"}},
		{"the admitted linter in another file", a + k + "nolint x/m.go errorlint # a reason\n", a, admit, []string{"nolint x/m.go errorlint: is in"}},
		{"another linter in the admitted file", a + k + "nolint x/k.go gosec # a reason\n", a, admit, []string{"nolint x/k.go gosec: is in"}},
		{"an admitted entry the list no longer holds", a, a, admit, []string{"nolint x/k.go errorlint: is admitted in admitted() of internal/exceptions_test.go, and suppressions.txt no longer holds it; delete the admission"}},
		{"the admitted entry in the base too", a + k, a + k, admit, nil},
	} {
		got := grown(tc.list, tc.base, tc.admit)
		if len(got) != len(tc.want) {
			t.Errorf("%s: grown = %q, want %d problems", tc.name, got, len(tc.want))
			continue
		}
		for i, w := range tc.want {
			if !strings.Contains(got[i], w) {
				t.Errorf("%s: %q, want it to say %q", tc.name, got[i], w)
			}
		}
	}
}

// TestASuppressionOfABanIsRefused (gates.exemptions.1, gragas#1750): a
// suppression that names depguard, forbidigo, gocritic or nolintlint, or that
// silences every linter, is refused in every spelling golangci-lint takes as
// one: a capital, a space after the colon or between names, a dash or an
// underscore in a name, a space for the colon. A nolint of another linter
// with its reason, and a comment that only starts with the word, are not.
func TestASuppressionOfABanIsRefused(t *testing.T) {
	t.Parallel()
	refused := map[string]string{
		"//nolint:forbidigo // a reason":                 "forbidigo",
		"//nolint:depguard // a reason":                  "depguard",
		"//nolint:gocritic // a reason":                  "gocritic",
		"//nolint:nolintlint // a reason":                "nolintlint",
		"//nolint:all // a reason":                       "all",
		"//nolint // a reason":                           "all",
		"//nolint every linter":                          "all",
		"// nolint:Forbidigo // a reason":                "forbidigo",
		"//nolint: forbidigo // a reason":                "forbidigo",
		"//nolint:errcheck, depguard // a reason":        "depguard",
		"//nolint:errcheck,go-critic,forbidigo // a why": "forbidigo",
	}
	for text, linter := range refused {
		_, bad := suppressions([]source{commentSource(t, text)})
		if !slices.ContainsFunc(bad, func(b string) bool { return strings.Contains(b, "a suppression of "+linter+" silences a ban") }) {
			t.Errorf("%q: bad = %q, want %s refused", text, bad, linter)
		}
	}
	for text, want := range map[string]string{
		"//nolint:err_check // a reason":    "nolint p/x.go err_check",
		"//nolint:go-critic // a reason":    "nolint p/x.go go-critic",
		"//nolint:ErrCheck // a reason":     "nolint p/x.go errcheck",
		"//nolint: gosec , noctx // reason": "nolint p/x.go gosec,noctx",
	} {
		lines, bad := suppressions([]source{commentSource(t, text)})
		if len(bad) != 0 || !slices.Equal(lines, []string{want}) {
			t.Errorf("%q: lines %q, bad %q, want the one line %q", text, lines, bad, want)
		}
	}
	for _, text := range []string{"// nolintComment is a name", "//nolintx:forbidigo", "// a nolint:forbidigo further in"} {
		if lines, bad := suppressions([]source{commentSource(t, text)}); len(lines)+len(bad) != 0 {
			t.Errorf("%q: read as a directive: %q %q", text, lines, bad)
		}
	}
}

// commentSource is a file of package p at p/x.go that holds the one comment.
func commentSource(t *testing.T, comment string) source {
	t.Helper()
	return parse(t, "p", "x.go", "package p\n\nfunc f() {\n\t_ = 1 "+comment+"\n}\n")
}
