package docs

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// The agent-instruction file held to agents.instructions.1 and .2: AGENTS.md
// is short, names no other repository, and every backticked path and make
// target in it resolves; CLAUDE.md only points to it. Like the document
// checks, it is a function of the text and of what exists, so it is also run
// with a breach planted.

const agentsMaxLines = 60

// otherRepositories are module paths of other repositories; the names of the private ones are
// not listed here, since the private names list is the only place they live.
var otherRepositories = regexp.MustCompile(`github\.com/[\w.-]+/`)

var agentsCommand = regexp.MustCompile(`^(make|go|git|gh) `)

// agentsProblems lists what is wrong with an instruction file. exists says
// whether a path from the root exists; targets are the Taskfile's tasks.
func agentsProblems(text string, exists func(string) bool, targets []string) []string {
	var out []string
	if n := strings.Count(text, "\n"); n > agentsMaxLines {
		out = append(out, fmt.Sprintf("AGENTS.md is %d lines, over %d", n, agentsMaxLines))
	}
	if m := otherRepositories.FindString(text); m != "" {
		out = append(out, "AGENTS.md names another repository: "+m)
	}
	for _, m := range backtick.FindAllStringSubmatch(text, -1) {
		tok := m[1]
		switch {
		case strings.ContainsAny(tok, "<>*{}$|=") || strings.HasPrefix(tok, "-"):
			// a placeholder, a pattern, a flag or a setting
		case strings.HasPrefix(tok, "task "):
			if t := strings.Fields(tok)[1]; !strings.HasPrefix(t, "-") && !slices.Contains(targets, t) {
				out = append(out, "the task "+t+" does not exist")
			}
		case agentsCommand.MatchString(tok):
			for w := range strings.FieldsSeq(tok) {
				if strings.HasPrefix(w, "./") && !exists(strings.TrimPrefix(w, "./")) {
					out = append(out, "the path "+w+" resolves to nothing")
				}
			}
		case pathLike.MatchString(tok) && (strings.Contains(tok, "/") || pathExt.MatchString(tok)):
			path := strings.TrimSuffix(tok, "/")
			if task, built := builtPaths()[path]; built {
				// Git ignores it and a task builds it: it resolves when that task is named, not when the folder
				// is there, so a tree that has not run the task is as green as one that has (gragas#2239).
				if !slices.Contains(targets, task) {
					out = append(out, "the path "+tok+" is built by the task "+task+", which does not exist")
				}
			} else if !exists(path) {
				out = append(out, "the path "+tok+" resolves to nothing")
			}
		}
	}
	return out
}

// builtPaths are the paths AGENTS.md may name that the tree does not hold: each is built by the task it maps
// to, and is ignored by git. A new one is a row here, with the task that builds it.
func builtPaths() map[string]string { return map[string]string{".tools/bin": "tools"} }

func makeTargets(t *testing.T) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, "Taskfile.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, m := range regexp.MustCompile(`(?m)^  ([a-z][a-z0-9-]*):$`).FindAllStringSubmatch(string(b), -1) {
		out = append(out, m[1])
	}
	return out
}

// rootExistsOn says whether a path from the root exists. In a worktree .git there is a file,
// and a path below it is in the common git dir (gragas#2221).
func rootExistsOn(t *testing.T) func(string) bool {
	t.Helper()
	common := commonGitDir(t, root)
	return func(rel string) bool {
		if below, ok := strings.CutPrefix(rel, ".git/"); ok && common != "" {
			_, err := os.Stat(filepath.Join(common, filepath.FromSlash(below)))
			return err == nil
		}
		_, err := os.Stat(filepath.Join(root, rel))
		return err == nil
	}
}

func TestAgentsMdIsAShortMapWhoseEveryPathResolves(t *testing.T) {
	t.Parallel()
	b, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range agentsProblems(string(b), rootExistsOn(t), makeTargets(t)) {
		t.Error(p)
	}
}

func TestClaudeMdOnlyPointsToAgentsMd(t *testing.T) {
	t.Parallel()
	// A regular file, not a link (gragas#872): its one line imports AGENTS.md.
	b, err := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "@AGENTS.md\n" {
		t.Errorf("CLAUDE.md is %q, not the one line that imports AGENTS.md", b)
	}
}

// TestABuiltPathResolvesByItsTask (gragas#2239): in a tree that has not run task tools, so where no path
// below .tools exists, AGENTS.md is green; and a built path whose task is gone from the Taskfile is red.
func TestABuiltPathResolvesByItsTask(t *testing.T) {
	t.Parallel()
	b, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	text, targets := string(b), makeTargets(t)
	if !strings.Contains(text, "`.tools/bin`") {
		t.Fatal("AGENTS.md names no .tools/bin: the plants below test nothing")
	}
	nothingBuilt := func(rel string) bool { return !strings.HasPrefix(rel, ".tools") && rootExistsOn(t)(rel) }
	if got := agentsProblems(text, nothingBuilt, targets); len(got) != 0 {
		t.Errorf("a tree with no .tools is red: %q", got)
	}
	without := slices.DeleteFunc(slices.Clone(targets), func(s string) bool { return s == builtPaths()[".tools/bin"] })
	got := agentsProblems(text, nothingBuilt, without)
	if !slices.ContainsFunc(got, func(s string) bool {
		return strings.Contains(s, ".tools/bin is built by the task tools, which does not exist")
	}) {
		t.Errorf("a Taskfile with no tools task says %q, want the built path named", got)
	}
}

func TestTheInstructionChecksFireOnAPlantedBreach(t *testing.T) {
	t.Parallel()
	b, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	good := string(b)
	targets, exists := makeTargets(t), rootExistsOn(t)
	tests := []struct{ name, text, says string }{
		{"a missing path", good + "\nSee `docs/nowhere.md`.\n", "docs/nowhere.md resolves to nothing"},
		{"a missing folder", good + "\nSee `nowhere/`.\n", "nowhere/ resolves to nothing"},
		{"a missing task", good + "\nRun `task nothing`.\n", "task nothing does not exist"},
		{"a built path that no task gives", good + "\nSee `.tools/nowhere`.\n", ".tools/nowhere resolves to nothing"},
		{"a missing path in a go command", good + "\nRun `go test ./nowhere`.\n", "./nowhere resolves to nothing"},
		{"a module path", good + "\nSee github.com/someone/else.\n", "names another repository"},
		{"too long", good + strings.Repeat("a line\n", agentsMaxLines), "over"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := agentsProblems(tc.text, exists, targets)
			if !slices.ContainsFunc(got, func(s string) bool { return strings.Contains(s, tc.says) }) {
				t.Errorf("said %q, want a problem saying %q", got, tc.says)
			}
		})
	}
}
