package docs

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// docs.freshness.4 and gates.measures.1: a duration is not typed into prose,
// where it goes stale; a number of seconds or minutes is read from the
// measured run record. The scan reads docs/*.md, AGENTS.md and
// CLAUDE.md, and the comments of the Makefile and of gate/*.yaml, and takes no
// code span or link, whose words are a command or an address. The lines that
// may stay are listed in gate/prose-durations.txt, one "path: line" a line,
// and the list only shrinks: gate/baselines.yaml counts it, and a line the
// prose no longer holds is itself a finding until it is taken out.

var (
	durationRe = regexp.MustCompile(`\b\d+(?:\.\d+)?\s?(?:s|secs?|seconds?|m|mins?|minutes?)\b`)
	comment    = regexp.MustCompile(`(?:^|\s)#(.*)$`)
	codeSpan   = regexp.MustCompile("`[^`\n]*`")
	linkText   = regexp.MustCompile(`\[[^\]\n]*\]\([^)\n]*\)|https?://\S+`)
)

// proseFiles are the files outside docs/ the scan reads, with the part of each
// it reads: all of a Markdown file, the comments of the others.
func readProse(t *testing.T) (map[string]string, []string) {
	t.Helper()
	prose := map[string]string{}
	paths := []string{"Taskfile.yml"}
	yamls, err := filepath.Glob(filepath.Join(root, "gate", "*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, y := range yamls {
		rel, _ := filepath.Rel(root, y)
		paths = append(paths, filepath.ToSlash(rel))
	}
	for _, p := range paths {
		b, err := os.ReadFile(filepath.Join(root, p))
		if err != nil {
			t.Fatal(err)
		}
		prose[p] = string(b)
	}
	b, err := os.ReadFile(filepath.Join(root, "gate", "prose-durations.txt"))
	if err != nil {
		t.Fatal(err)
	}
	var stays []string
	for l := range strings.SplitSeq(string(b), "\n") {
		if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "#") {
			stays = append(stays, l)
		}
	}
	return prose, stays
}

// spoken is the words of a line the scan reads: for a file that is not
// Markdown only what follows a "#" that starts a word, and for every file without its code spans
// and links.
func spoken(path, line string) string {
	if !strings.HasSuffix(path, ".md") {
		m := comment.FindStringSubmatch(line)
		if m == nil {
			return ""
		}
		line = m[1]
	}
	return linkText.ReplaceAllString(codeSpan.ReplaceAllString(line, ""), "")
}

// durationLines are the lines of text that say a duration, as "path: line".
func durationLines(path, text string) []string {
	var out []string
	fenced := false
	for l := range strings.SplitSeq(text, "\n") {
		if strings.HasSuffix(path, ".md") && strings.HasPrefix(strings.TrimSpace(l), "```") {
			fenced = !fenced
			continue
		}
		if !fenced && durationRe.MatchString(spoken(path, l)) {
			out = append(out, path+": "+strings.TrimSpace(l))
		}
	}
	return out
}

// durations is docs.freshness.4: a line that says a duration and is not in the
// baseline is a finding, and so is a baseline line the prose no longer holds.
func durations(w docWorld) []string {
	var found []string
	for name, text := range w.docs {
		if !strings.Contains(strings.TrimPrefix(name, "docs/"), "/") {
			found = append(found, durationLines(name, text)...)
		}
	}
	for name, text := range w.prose {
		found = append(found, durationLines(name, text)...)
	}
	var out []string
	for _, l := range found {
		if !slices.Contains(w.stays, l) {
			p, text, _ := strings.Cut(l, ": ")
			out = append(out, p+": a duration in prose: "+text+" (the measured time is in the run record; a line that must stay goes in gate/prose-durations.txt)")
		}
	}
	for _, l := range w.stays {
		if !slices.Contains(found, l) {
			out = append(out, l+": is not in the prose any more: take the line out of gate/prose-durations.txt")
		}
	}
	slices.Sort(out)
	return out
}

// withStays is w with one more line in the baseline.
func withStays(w docWorld, line string) docWorld {
	w.stays = append(slices.Clone(w.stays), line)
	return w
}
