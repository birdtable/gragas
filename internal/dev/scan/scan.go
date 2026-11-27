// Package scan is the domain of the supply scans of make check (M0a-10,
// architecture 14): the workflow audit, the secret scan over the tree and
// the history, the known-vulnerability scan, the licence check and the
// forbidden-name scan. It reads what each pinned tool prints into findings
// and says which of them fails.
//
// Each scan proves its tools on this host before it reads the tree: a plant
// the scan must turn red, a clean control it must leave green, and then the
// real input, whose count it prints and which may not be empty
// (gates.plants.1, gates.plants.5). A tool that is missing, that cannot
// start or that prints what the scan cannot read fails the scan: nothing
// fails open.
//
// It reaches no disk and starts no program: the command hands in a Machine.
package scan

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Result is what one run of a program gave back.
type Result struct {
	// Code is the exit status.
	Code int
	// Stdout and Stderr are the two streams, whole.
	Stdout, Stderr []byte
}

// Machine is what a scan may ask of the host. The command fills it; a test
// hands in its own.
type Machine struct {
	// Temp is a new empty folder that the command removes when the scan ends.
	Temp func(ctx context.Context) (string, error)
	// Write writes data at path, an absolute path under a folder Temp gave,
	// making the folders on the way.
	Write func(path string, data []byte) error
	// Run runs prog, an absolute path that Tool or Git gave, in dir, the
	// tree when dir is "", with args and the extra env lines, and returns
	// its result. An error is a program that could not start, never an exit
	// status.
	Run func(ctx context.Context, dir string, env []string, prog string, args ...string) (Result, error)
	// Tool is the absolute path of a pinned tool of tools.yaml, or an error
	// saying it is missing and how to get it.
	Tool func(name string) (string, error)
	// Git is the absolute path of git, as the gate starts it.
	Git func() (string, error)
	// Go is the absolute path of go, as the gate starts it.
	Go func() (string, error)
	// DepNotice is the NOTICE file a library ships beside its licence file,
	// by the licence file's absolute path, or "" when it ships none. It
	// reads only in the module cache and the scan's temp folders.
	DepNotice func(ctx context.Context, licence string) (string, error)
	// Tracked lists the tracked files the tree holds, relative to it: a
	// tracked file deleted from the tree is left out.
	Tracked func(ctx context.Context) ([]string, error)
	// CopyUnignored copies every file git does not ignore in dir, the tree
	// when dir is "", tracked or not, as it is on disk, into a new temp
	// folder, and answers the folder and how many files it copied. A
	// tracked file deleted from disk and a name that is not a regular file
	// are left out. No excludes file of the host's config hides a file.
	CopyUnignored func(ctx context.Context, dir string) (string, int, error)
	// TrackedFiles reads every file git tracks in dir, the tree when dir is
	// "", as it is on disk, by its path relative to dir; a link is read as
	// its target, which is what git stores. A tracked file deleted from disk
	// and a name that is neither a file nor a link are left out. dir is the
	// tree or a temp folder of this scan.
	TrackedFiles func(ctx context.Context, dir string) (map[string][]byte, error)
	// NamesList is the path NamesHashesVar names, absolute and clean, and the
	// file's bytes, or "" and nil when it is unset. An error is a path that
	// is not absolute and clean, or a file that cannot be read.
	NamesList func() (string, []byte, error)
	// Exists is whether dir, the tree when dir is "" or a temp folder of this
	// scan, holds name at its root, of any kind, as the file system finds it:
	// on a host that ignores case, a name that differs only by case is the
	// name, as it is to a tool that opens it there.
	Exists func(dir, name string) (bool, error)
	// GitFile is whether the repository in dir, the tree when dir is "" or a
	// temp folder of this scan, holds rel in its git folder, of any kind: rel
	// as git rev-parse --git-path names it, so a worktree's is its common
	// folder's where git keeps it there.
	GitFile func(ctx context.Context, dir, rel string) (bool, error)
	// Read reads a file of the tree, by its path relative to it.
	Read func(path string) ([]byte, error)
	// Index is the index file the commit being made is built from, as the
	// hook that runs the gate was handed it in GIT_INDEX_FILE, absolute; ""
	// when it is unset, and git's own index is the one.
	Index func() (string, error)
	// Files lists every file under dir, a temp folder of this scan, by its
	// path relative to dir with / between the parts, as the file system
	// spells it.
	Files func(dir string) ([]string, error)
	// Root is the tree's absolute path, for a tool that takes a URL.
	Root string
	// GitEnv is the extra env lines every git of a scan runs with: it makes
	// git read no config of the host's, no hooks, no signing, no user, and
	// honour no replace ref, so it shows the commits a push sends. The
	// command builds it, since the scan knows no system's null file.
	GitEnv []string
	// Within is whether the path p is root or lies below it, by the host's
	// own rules for a path, both already lower-cased. nil says no path is.
	Within func(root, p string) bool
	// Now is the time on this host's clock.
	Now func() time.Time
}

// CountLine is the last line a scan prints when it passes (gragas#695,
// gates.measures.6): what it read, n of unit, which the gate runner holds to
// the member's floor in gate/counts.yaml. A scan that read nothing has
// failed on ErrEmpty before it gets here.
func CountLine(n int, unit string) string { return fmt.Sprintf("count: %d %s", n, unit) }

// Case is one run a scan must judge as its plant, its clean control or its
// refusal case, before it judges the tree.
type Case struct {
	// Name says what the case holds, in the words a failure prints.
	Name string
	// Red is the verdict the case must reach: true for a plant or a refusal
	// case, false for a clean control.
	Red bool
	// Judge runs the case and returns whether the scan found it red.
	Judge func(ctx context.Context) (bool, error)
}

// ErrProof is a case that did not reach its verdict: the scan cannot be
// trusted on this host, so it fails before it reads the tree.
var ErrProof = errors.New("the scan did not prove itself on its plant and control")

// Prove runs each case in turn and returns a line per case, or an error
// naming the first case that failed to run or reached the wrong verdict.
func Prove(ctx context.Context, scan string, cases ...Case) ([]string, error) {
	lines := make([]string, 0, len(cases))
	for _, c := range cases {
		red, err := c.Judge(ctx)
		if err != nil {
			return lines, fmt.Errorf("%s: %s: %w: %w", scan, c.Name, ErrProof, err)
		}
		if red != c.Red {
			return lines, fmt.Errorf("%s: %s: %w: it was %s, and it must be %s", scan, c.Name, ErrProof, word(red), word(c.Red))
		}
		lines = append(lines, fmt.Sprintf("  ok  %s: %s", word(red), c.Name))
	}
	return lines, nil
}

// word is a verdict in the words a line prints.
func word(red bool) string {
	if red {
		return "red"
	}
	return "green"
}

// Refusal is a scan that failed on the tree. Its message names the scan's
// home and the fix (gates.quality.2).
type Refusal struct {
	// Scan is the scan that refused.
	Scan string
	// Findings are the lines that say what it found, one per finding.
	Findings []string
	// Home is where the rule lives, a file or a command.
	Home string
	// Fix is what a person does about it.
	Fix string
}

func (r *Refusal) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s: %d %s\n", r.Scan, len(r.Findings), plural(len(r.Findings), "finding", "findings"))
	for _, f := range r.Findings {
		b.WriteString("  " + f + "\n")
	}
	fmt.Fprintf(&b, "  home: %s\n  fix: %s", r.Home, r.Fix)
	return b.String()
}

// plural is one or many, by n.
func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// ErrEmpty is a scan given no input: it fails closed, since a scan of
// nothing proves nothing (gates.plants.5).
var ErrEmpty = errors.New("no input to scan")

// rewrites is a finding for each way the repository in dir would show git
// a history other than the one a push of it sends: a ref under
// refs/replace/, which git shows in place of the commit it names, a grafts
// file, which gives a commit other parents, and a shallow clone, whose git
// shows a commit it holds without parents it does not. A scan refuses each
// by name before it reads history: that is plainer than undoing them, and
// a count of commits cannot see the shallow cut, as git counts the same
// cut history. A replace ref is named by the commit it replaces; one whose
// name is not a commit's id is not printed, as it may hold a name.
func rewrites(ctx context.Context, m Machine, git, dir string) ([]string, error) {
	r, err := m.Run(ctx, dir, m.GitEnv, git, "for-each-ref", "--format=%(refname)", "refs/replace/")
	if err != nil {
		return nil, err
	}
	if r.Code != 0 {
		return nil, fmt.Errorf("git for-each-ref exited %d: %s", r.Code, firstLine(r.Stderr))
	}
	var found []string
	for ref := range strings.FieldsSeq(string(r.Stdout)) {
		if !commitID(strings.TrimPrefix(ref, "refs/replace/")) {
			ref = "a ref under refs/replace/ whose name is not a commit's id"
		}
		found = append(found, ref+": git shows another commit in place of the one it names, and the scan refuses it: delete it with git replace -d")
	}
	grafts, err := m.GitFile(ctx, dir, "info/grafts")
	if err != nil {
		return nil, err
	}
	if grafts {
		found = append(found, "info/grafts in the git folder: git gives a commit other parents than it has, and the scan refuses it: delete the file")
	}
	r, err = m.Run(ctx, dir, m.GitEnv, git, "rev-parse", "--is-shallow-repository")
	if err != nil {
		return nil, err
	}
	if r.Code != 0 {
		return nil, fmt.Errorf("git rev-parse --is-shallow-repository exited %d: %s", r.Code, firstLine(r.Stderr))
	}
	if strings.TrimSpace(string(r.Stdout)) != "false" {
		found = append(found, "the clone is shallow: git shows its oldest commits without their parents, and the scan refuses it: run git fetch --unshallow")
	}
	return found, nil
}

// commitID is whether id is a commit's full id as git prints it: 40 or 64
// lowercase hex digits, for SHA-1 or SHA-256.
func commitID(id string) bool {
	return (len(id) == 40 || len(id) == 64) && strings.Trim(id, "0123456789abcdef") == ""
}
