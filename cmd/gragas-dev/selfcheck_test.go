package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeIssues answers the state of each issue it is given and an error for any other, as GitHub answers 404.
func fakeIssues(states map[int]string) issueState {
	return func(_ context.Context, n int) (string, error) {
		if s, ok := states[n]; ok {
			return s, nil
		}
		return "", errors.New("gh api: Not Found (HTTP 404)")
	}
}

const selfDoc = "# Self-check\n\n## What `gragas check` finds in gragas\n\n| Rule | Key | Count | Issue | What is unmet | Exemption |\n|---|---|---|---|---|---|\n" +
	"| gates.command.1 | pre-push empty | 1 | #1378 | x | e |\n| gates.command.1 | pre-push build linux/amd64 | 1 | #1376 | x | e |\n| go.cycle.2 | pre-push lint linux/amd64 | 1 | #1376 | x | e |\n\n## Releases\n"

// TestSelfCheckIssuesIsRedOnAClosedOrAMissingIssueAndOnABadRow (gragas#1294): the release tier's member asks
// whether each issue a row names is open, once for each distinct issue. Planted: a closed issue, one GitHub
// has none of, a row with no #N and a document with no table, each red; all open is green.
func TestSelfCheckIssuesIsRedOnAClosedOrAMissingIssueAndOnABadRow(t *testing.T) {
	t.Parallel()
	open := map[int]string{1376: "open", 1378: "open"}
	if problems, n := openIssues(t.Context(), selfDoc, fakeIssues(open)); len(problems) != 0 || n != 2 {
		t.Errorf("all open: problems %v, %d issues, want none and 2", problems, n)
	}
	for name, c := range map[string]struct {
		doc    string
		states map[int]string
		want   string
	}{
		"a closed issue":      {selfDoc, map[int]string{1376: "open", 1378: "closed"}, "#1378 is closed"},
		"a missing issue":     {selfDoc, map[int]string{1378: "open"}, "#1376 is not an issue GitHub answers for"},
		"a row with no #N":    {strings.Replace(selfDoc, "#1378", "later", 1), open, "names no issue as #N"},
		"a row of five cells": {strings.Replace(selfDoc, " | #1378", "", 1), open, "names no issue as #N"},
		"no table":            {"# Self-check\n", open, "has no section"},
		"no row":              {"## What `gragas check` finds in gragas\n\n| Rule | Key | Count | Issue | What is unmet | Exemption |\n|---|---|---|---|---|---|\n", open, "has no row"},
	} {
		problems, _ := openIssues(t.Context(), c.doc, fakeIssues(c.states))
		if len(problems) != 1 || !strings.Contains(problems[0], c.want) {
			t.Errorf("%s: problems %v, want one with %q", name, problems, c.want)
		}
	}
	asked := map[int]int{}
	counting := func(_ context.Context, n int) (string, error) { asked[n]++; return "open", nil }
	if _, n := openIssues(t.Context(), selfDoc, counting); n != 2 || asked[1376] != 1 {
		t.Errorf("an issue two rows name was asked %d times, and %d issues counted; want once and 2", asked[1376], n)
	}
}

// TestTheRealSelfCheckTableNamesIssues: the table of docs/self-check.md has rows the command reads, so the
// release's member is not green over a table it cannot find.
func TestTheRealSelfCheckTableNamesIssues(t *testing.T) {
	t.Parallel()
	doc, err := os.ReadFile(filepath.Join("..", "..", "docs", "self-check.md"))
	if err != nil {
		t.Fatal(err)
	}
	if problems, n := openIssues(t.Context(), string(doc), func(context.Context, int) (string, error) { return "open", nil }); len(problems) != 0 || n == 0 {
		t.Errorf("the real table: problems %v, %d issues, want none and at least one", problems, n)
	}
}

// TestSelfCheckWithAnotherWordIsUsage: only issues is a subcommand.
func TestSelfCheckWithAnotherWordIsUsage(t *testing.T) {
	t.Parallel()
	var out, errb strings.Builder
	if code := runSelfCheck(t.Context(), []string{"other"}, &out, &errb); code != exitUsage || !strings.Contains(errb.String(), "usage: gragas-dev self-check issues") {
		t.Errorf("code %d, said %q", code, errb.String())
	}
}
