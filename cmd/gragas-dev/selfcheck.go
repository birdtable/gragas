package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/birdtable/gragas/internal/dev/say"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const selfCheckUsage = `usage: gragas-dev self-check issues [--repo owner/name] [-C dir]
`

// selfCheckHead is the section of docs/self-check.md whose rows excuse the findings of gragas check on
// gragas (gragas#1294): the Issue column of its table names the open issue that closes each.
const selfCheckHead = "## What `gragas check` finds in gragas"

// rowIssue is a cell that is an issue number: #N.
var rowIssue = regexp.MustCompile(`^#([1-9][0-9]*)$`)

// runSelfCheck is the self-check command. issues reads the Issue column of the table of docs/self-check.md
// and asks GitHub, through gh, whether each issue is open: it is red on a closed one, on one GitHub does
// not have, on a row that names none, and on a document with no table. It is the release tier's member:
// the commit and the push read no network, and the offline half of the same rule, that findings and rows
// are equal, is TestGragasAuditsItself.
func runSelfCheck(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	talk := &say.Speaker{W: stderr}
	if len(args) == 0 || args[0] != "issues" {
		talk.Text(selfCheckUsage)
		return exitUsage
	}
	fl := flag.NewFlagSet("self-check issues", flag.ContinueOnError)
	fl.SetOutput(stderr)
	repo := fl.String("repo", "birdtable/gragas", "the repository whose issues the rows name")
	dir := fl.String("C", ".", "the repository holding docs/self-check.md")
	if err := fl.Parse(args[1:]); err != nil || fl.NArg() != 0 {
		talk.Text(selfCheckUsage)
		return exitUsage
	}
	doc, err := os.ReadFile(filepath.Join(*dir, "docs", "self-check.md"))
	if err != nil {
		talk.Text("gragas-dev self-check issues: " + err.Error() + "\n")
		return exitFailed
	}
	problems, n := openIssues(ctx, string(doc), ghIssueState(*repo))
	for _, p := range problems {
		talk.Text("self-check: " + p + "\n")
	}
	if len(problems) > 0 {
		return exitFailed
	}
	(&say.Speaker{W: stdout}).Text(fmt.Sprintf("self-check: the %d issues the rows of docs/self-check.md name are open\n", n))
	return exitDone
}

// issueState answers the state of an issue, "open" or "closed", or an error when GitHub has none.
type issueState func(ctx context.Context, n int) (string, error)

// openIssues reads the Issue column of the table in doc and answers each problem and how many distinct
// issues the rows name. A problem is a closed issue, an issue the state call cannot answer, a row whose
// issue cell is not #N, and a document with no table or no row.
func openIssues(ctx context.Context, doc string, state issueState) (problems []string, issues int) {
	_, section, ok := strings.Cut(doc, selfCheckHead+"\n")
	if !ok {
		return []string{"docs/self-check.md has no section " + selfCheckHead}, 0
	}
	if i := strings.Index(section, "\n## "); i >= 0 {
		section = section[:i]
	}
	seen := map[int]bool{}
	for line := range strings.SplitSeq(section, "\n") {
		if !strings.HasPrefix(line, "| ") || strings.HasPrefix(line, "| Rule |") || strings.HasPrefix(line, "|---") {
			continue
		}
		cells := strings.Split(strings.Trim(line, "| "), " | ")
		var m []string
		if len(cells) == 6 {
			m = rowIssue.FindStringSubmatch(cells[3])
		}
		if m == nil {
			problems = append(problems, "a row names no issue as #N: "+line)
			continue
		}
		n, err := strconv.Atoi(m[1])
		if err != nil {
			problems = append(problems, "a row's issue is not a number: "+line)
			continue
		}
		if seen[n] {
			continue
		}
		seen[n] = true
		switch got, err := state(ctx, n); {
		case err != nil:
			problems = append(problems, fmt.Sprintf("#%d is not an issue GitHub answers for: %v", n, err))
		case got != "open":
			problems = append(problems, fmt.Sprintf("#%d is %s, so its rows go with the gap it closed, or the issue is opened again", n, got))
		}
	}
	if len(seen) == 0 && len(problems) == 0 {
		problems = append(problems, "the table of "+selfCheckHead+" has no row")
	}
	return problems, len(seen)
}

// ghIssueState asks GitHub through the gh program, which holds its own login: the command carries no
// token and prints none. A call that fails is the error with gh's first line.
func ghIssueState(repo string) issueState {
	return func(ctx context.Context, n int) (string, error) {
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		out, err := withArgs(exec.CommandContext(ctx, "gh"), []string{"api", "repos/" + repo + "/issues/" + strconv.Itoa(n), "--jq", ".state"}).CombinedOutput()
		if err != nil {
			first, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
			return "", fmt.Errorf("gh api: %s", first)
		}
		return strings.TrimSpace(string(out)), nil
	}
}
