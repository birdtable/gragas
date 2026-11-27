package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/birdtable/gragas/internal/dev/say"
	"io"
	"regexp"
	"slices"
	"strings"
	"time"
)

// landingTimeout ends a task landing that hangs (gragas#2263). It is a
// backstop and no budget, longer than a hook's: the landing runs the whole of
// go test ./..., one package of which took 48 minutes under load (gragas#2340's
// heavy run), and its budget is hatch's check after the run.
const landingTimeout = 3 * time.Hour

const landingUsage = `usage: gragas-dev landing run [--base branch] [-C dir]
       gragas-dev landing begin [--base branch] [-C dir]
       gragas-dev landing check --tested sha [--base branch] [-C dir]
       gragas-dev landing merge N --head sha --tested sha [-C dir]

  run    the clone's .tools/bin/task landing, as the hooks run their tiers: with the hooks' allowlist of
         the environment, so no variable that switches a tool of the tiers reaches it (TASK_DRY greens the
         run unrun; gragas#2169, gragas#2181). begin, check and merge refuse while one is set
  begin  print the base this tree was merged with (origin/<branch>, main unless --base says another) and
         refuse a tree that does not hold it: the landing runs on a tree merged with the base it names
  check  fetch the base and refuse, naming both, when its tip is not the sha --tested names
  merge  read the pull request's own base (main, or release-1), check it as check does, and then, in the same
         process, gh pr merge N --squash --match-head-commit sha; the lander passes the sha begin printed
         and merges with nothing else
`

// landingBranch is a branch name as it is fetched: no option, no space, no ref syntax beyond a slash.
var landingBranch = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,100}$`)

// landingFull is what --tested and --head hold: a full commit name, so no abbreviation stands for another commit.
var landingFull = regexp.MustCompile(`^[0-9a-f]{40}$`)

// landingNumber is a pull request's number: gh reads a name as a branch selector, so digits alone reach it.
var landingNumber = regexp.MustCompile(`^[0-9]{1,9}$`)

// landingGH is what the landing asks of GitHub: the base branch a pull request merges into, and the merge itself.
// main hands in ghOver; a test its own.
type landingGH struct {
	base  func(ctx context.Context, dir, number string) (string, error)
	merge func(ctx context.Context, dir, number, head string) error
}

// ghOver is the landing's GitHub: gh, started as every program of this command is (outputIn, through proc.Run, gragas#2263),
// with the environment gragas-dev was started with as its whole one, which gh reads its sign-in from.
func ghOver(environ []string) landingGH {
	run := func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		return outputIn(ctx, dir, environ, "gh", args...)
	}
	return landingGH{
		base: func(ctx context.Context, dir, number string) (string, error) {
			out, err := run(ctx, dir, "pr", "view", number, "--json", "baseRefName", "--jq", ".baseRefName")
			return strings.TrimSpace(string(out)), err
		},
		merge: func(ctx context.Context, dir, number, head string) error {
			_, err := run(ctx, dir, "pr", "merge", number, "--squash", "--match-head-commit", head)
			return err
		},
	}
}

// runLanding is the landing command (gragas#1819). A lander tests the pull request merged with its base as it was
// when the run started, and a second lander may merge in between; gh's --match-head-commit pins the head and
// never the base. begin names the base the tree holds (an ancestor of HEAD, nothing stored). check and merge
// take that sha from the lander, so the base compared is the run's and never the tree of the moment, and
// merge does the check and the merge in one process, on the base the pull request names. run is the landing run
// itself, given environ cut to the hooks' allowlist (allowed); the other words refuse an environ that holds a tool's
// switch, so a lander who
// ran task landing under it, not run, is stopped before the merge.
func runLanding(ctx context.Context, args, environ []string, gh landingGH, stdout, stderr io.Writer) int {
	talk := &say.Speaker{W: stderr}
	out := &say.Speaker{W: stdout}
	a, ok := landingParse(args, stderr)
	if !ok {
		talk.Text(landingUsage)
		return exitUsage
	}
	fail := func(msg string) int {
		talk.Text("gragas-dev landing: " + msg + "\n")
		return exitFailed
	}
	if a.word == "run" {
		h, err := newHook(ctx, a.dir, environ, stdout, stderr)
		if err != nil {
			return fail(err.Error())
		}
		if !landingBranch.MatchString(a.base) {
			return fail("refused: the base " + a.base + " is not a branch name")
		}
		// The base goes to the task as a task variable, never an environment variable (gragas#2391); the
		// Taskfile's landing begin defaults it to main, so a run on main passes none.
		task := []string{"landing"}
		if a.base != "main" {
			task = append(task, "LANDING_BASE="+a.base)
		}
		return h.runTask(ctx, h.root, h.env, landingTimeout, task...)
	}
	if i := slices.IndexFunc(environ, isSwitch); i >= 0 {
		k, _, _ := strings.Cut(environ[i], "=")
		return fail("refused: " + k + " is set, which a tool of the landing obeys (gragas#2161): unset it, and run the landing again by gragas-dev landing run")
	}
	if a.word == "merge" {
		var err error
		if a.base, err = gh.base(ctx, a.dir, a.number); err != nil {
			return fail("the pull request's base is not read, so the merge is refused: " + err.Error())
		}
	}
	if !landingBranch.MatchString(a.base) {
		return fail("refused: the base " + a.base + " is not a branch name")
	}
	if a.word == "begin" {
		if msg := landingBegin(ctx, environ, a.dir, a.base, out); msg != "" {
			return fail(msg)
		}
		return exitDone
	}
	if msg := landingCompare(ctx, environ, a); msg != "" {
		return fail(msg)
	}
	if a.word == "check" {
		out.Text("landing: tested on " + a.base + " " + a.tested + ", still origin/" + a.base + "\n")
		return exitDone
	}
	if err := gh.merge(ctx, a.dir, a.number, a.head); err != nil {
		return fail("the merge failed: " + err.Error())
	}
	out.Text("landing: merged " + a.number + " at " + a.head + ", tested on " + a.base + " " + a.tested + "\n")
	return exitDone
}

// landingArgs are the words and flags of a landing command.
type landingArgs struct {
	word, number, dir, tested, head, base string
}

// landingParse reads the command line, false when it is not one: no word, a merge with no number, or a check or
// merge without the full shas it needs.
func landingParse(args []string, stderr io.Writer) (landingArgs, bool) {
	var a landingArgs
	if len(args) == 0 || !slices.Contains([]string{"run", "begin", "check", "merge"}, args[0]) {
		return a, false
	}
	a.word, args = args[0], args[1:]
	if a.word == "merge" {
		if len(args) == 0 || !landingNumber.MatchString(args[0]) {
			return a, false
		}
		a.number, args = args[0], args[1:]
	}
	fl := flag.NewFlagSet("landing", flag.ContinueOnError)
	fl.SetOutput(stderr)
	fl.StringVar(&a.dir, "C", ".", "the repository")
	fl.StringVar(&a.tested, "tested", "", "the base the landing ran on, as begin printed it")
	fl.StringVar(&a.head, "head", "", "the pull request's head commit")
	fl.StringVar(&a.base, "base", "main", "the branch the pull request merges into")
	if err := fl.Parse(args); err != nil || fl.NArg() != 0 {
		return a, false
	}
	if a.word == "begin" || a.word == "run" {
		return a, true
	}
	return a, landingFull.MatchString(a.tested) && (a.word == "check" || landingFull.MatchString(a.head))
}

// landingCompare fetches the base and says why the merge is refused: it could not be fetched, or its tip is not the
// sha the landing ran on.
func landingCompare(ctx context.Context, environ []string, a landingArgs) string {
	if _, err := output(ctx, environ, a.dir, nil, "git", "fetch", "--quiet", "origin", "+refs/heads/"+a.base+":"+landingRef(a.base)); err != nil {
		return a.base + " is not fetched, so the merge is refused: " + err.Error()
	}
	now, err := landingRev(ctx, environ, a.dir, a.base)
	if err != nil {
		return err.Error()
	}
	if now != a.tested {
		return fmt.Sprintf("refused: tested on %s %s, origin/%s is now %s: run the landing again on the new merged tree", a.base, a.tested, a.base, now)
	}
	return ""
}

// landingBegin says the base the tree holds, or why it refuses: the tree holds that base as an ancestor of HEAD, or it
// was never merged with it.
func landingBegin(ctx context.Context, environ []string, dir, base string, out *say.Speaker) string {
	held, err := landingRev(ctx, environ, dir, base)
	if err != nil {
		return err.Error()
	}
	if _, err := output(ctx, environ, dir, nil, "git", "merge-base", "--is-ancestor", held, "HEAD"); err != nil {
		return "refused: the tree does not hold origin/" + base + " " + held + ": merge it into the tree and run the landing again"
	}
	out.Text("landing: the tree is merged with " + base + " " + held + "\n")
	return ""
}

// landingRef is the remote-tracking branch named in full, so a tag called origin/main is never read (gragas#589).
func landingRef(base string) string { return "refs/remotes/origin/" + base }

// landingRev is the commit the remote-tracking base names.
func landingRev(ctx context.Context, environ []string, dir, base string) (string, error) {
	got, err := output(ctx, environ, dir, nil, "git", "rev-parse", "--verify", "--quiet", landingRef(base)+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("origin/%s is not known to this clone: %w", base, err)
	}
	return strings.TrimSpace(string(got)), nil
}
