package internal

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// attrRule is one line of .gitattributes as far as the text attribute goes: the files it names and what it says
// of text: "set" (text), "unset" (-text), "auto" (text=auto) or "unspecified" (!text).
type attrRule struct {
	pattern string
	re      *regexp.Regexp
	text    string
}

// attrPattern is a .gitattributes pattern as a regexp over a slash path: a pattern with no slash names a file
// of any folder, one with a slash is anchored at the root, * stops at a slash and ** does not.
func attrPattern(p string) *regexp.Regexp {
	anchored := strings.Contains(strings.TrimPrefix(p, "/"), "/")
	p = strings.TrimPrefix(p, "/")
	r := regexp.QuoteMeta(p)
	r = strings.ReplaceAll(r, `\*\*`, "\x00")
	r = strings.ReplaceAll(r, `\*`, `[^/]*`)
	r = strings.ReplaceAll(r, `\?`, `[^/]`)
	r = strings.ReplaceAll(r, "\x00", `.*`)
	if anchored {
		return regexp.MustCompile("^" + r + "$")
	}
	return regexp.MustCompile("(^|/)" + r + "$")
}

// parseAttributes reads the text rules of a .gitattributes, in order.
func parseAttributes(data string) []attrRule {
	var out []attrRule
	for line := range strings.Lines(data) {
		f := strings.Fields(line)
		if len(f) < 2 || strings.HasPrefix(f[0], "#") {
			continue
		}
		state := ""
		for _, a := range f[1:] {
			switch a {
			case "text":
				state = "set"
			case "-text":
				state = "unset"
			case "!text":
				state = "unspecified"
			case "text=auto":
				state = "auto"
			}
		}
		if state != "" {
			out = append(out, attrRule{pattern: f[0], re: attrPattern(f[0]), text: state})
		}
	}
	return out
}

// textOf is what the last rule that names p says of its text attribute, "unspecified" when none does.
func textOf(rules []attrRule, p string) string {
	state := "unspecified"
	for _, r := range rules {
		if r.re.MatchString(p) {
			state = r.text
		}
	}
	return state
}

// hasLine says whether a line of data holds exactly the fields given.
func hasLine(data string, fields ...string) bool {
	for line := range strings.Lines(data) {
		if slices.Equal(strings.Fields(line), fields) {
			return true
		}
	}
	return false
}

// hashedPaths returns the paths whose bytes a digest is taken over, which no conversion may touch (-text):
// the crash journey's seeds and goldens, the vendored font licences and the fixtures the lock files sum.
func hashedPaths() []string {
	return []string{
		"internal/journeys/testdata/history/x",
		"internal/journeys/testdata/v2/x",
		"internal/journeys/testdata/crash-x.golden",
		"explorer/packages/tokens/fonts/ibm-plex-mono/LICENSE.txt",
		"explorer/packages/tokens/fonts/PROVENANCE.json",
		"internal/host/assets/fixture/projects/clean/.gragas/lock.yaml",
		"internal/host/assets/fixture/base/bindings/std-errors-1.yaml",
	}
}

// attributesProblems holds a .gitattributes to gragas#1609: one rule for the whole repository, * text=auto eol=lf,
// so every text file is checked out with \n whatever core.autocrlf a clone has; each path of hashed stays -text; and no
// tracked text file holds a carriage return, so `git add --renormalize .` changes no tracked byte. files are the
// tracked paths and read gives one's bytes. A file list that is empty is no subject: it is refused.
func attributesProblems(attrs string, files []string, read func(rel string) ([]byte, error), hashed []string) ([]string, error) {
	if len(files) == 0 {
		return nil, errors.New("could not run: no tracked file was listed")
	}
	var out []string
	if !hasLine(attrs, "*", "text=auto", "eol=lf") {
		out = append(out, ".gitattributes has no whole-repository line `* text=auto eol=lf`")
	}
	rules := parseAttributes(attrs)
	for _, p := range hashed {
		if got := textOf(rules, p); got != "unset" {
			out = append(out, p+": text is "+got+", not unset: a digest is taken over its bytes, so it needs -text")
		}
	}
	for _, f := range files {
		if textOf(rules, f) == "unset" {
			continue
		}
		data, err := read(f)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, err
		}
		if bytes.IndexByte(data, 0) < 0 && bytes.Contains(data, []byte("\r\n")) {
			out = append(out, f+": holds a carriage return and is not -text, so a renormalize would change a tracked byte")
		}
	}
	return out, nil
}

// TestGitattributesKeepsOneRuleForTheWholeRepo holds gragas#1609: the whole-repo line ending rule is in
// .gitattributes, each path a digest is taken over stays -text, and no tracked text file has a CRLF in the
// index, which is what `git add --renormalize .` would change.
func TestGitattributesKeepsOneRuleForTheWholeRepo(t *testing.T) {
	t.Parallel()
	problems, err := attributesProblems(read(t, ".gitattributes"), trackedIn(t, root), func(rel string) ([]byte, error) {
		return os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	}, hashedPaths())
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestGitattributesPlanted plants each way it goes wrong: the whole-repo line dropped; a hashed path with no
// -text; a text file with a CRLF that no -text line covers; and no file listed at all, which is refused.
func TestGitattributesPlanted(t *testing.T) {
	t.Parallel()
	attrs := read(t, ".gitattributes")
	files := []string{"a.txt", "b/c.golden"}
	content := map[string]string{"a.txt": "one\ntwo\n", "b/c.golden": "x\r\ny\r\n"}
	reader := func(rel string) ([]byte, error) {
		if s, ok := content[rel]; ok {
			return []byte(s), nil
		}
		return nil, os.ErrNotExist
	}
	hashed := []string{"b/c.golden"}
	withGolden := attrs + "b/*.golden -text\n"

	if got, err := attributesProblems(withGolden, files, reader, hashed); err != nil || len(got) != 0 {
		t.Errorf("a clean plant is red: %q %v", got, err)
	}
	got, err := attributesProblems(strings.ReplaceAll(withGolden, "* text=auto eol=lf", "# dropped"), files, reader, hashed)
	if err != nil {
		t.Fatal(err)
	}
	fires(t, got, "no whole-repository line")
	if got, err = attributesProblems(attrs, files, reader, hashed); err != nil {
		t.Fatal(err)
	}
	fires(t, got, "b/c.golden: text is auto, not unset")
	fires(t, got, "b/c.golden: holds a carriage return and is not -text")
	content["a.txt"] = "one\r\ntwo\r\n"
	if got, err = attributesProblems(withGolden, files, reader, hashed); err != nil {
		t.Fatal(err)
	}
	fires(t, got, "a.txt: holds a carriage return and is not -text")
	// A binary file with a CRLF in it is not text, whatever git would guess.
	content["a.txt"] = "\x00\r\n"
	if got, err = attributesProblems(withGolden, files, reader, hashed); err != nil || len(got) != 0 {
		t.Errorf("a binary file was called text: %q %v", got, err)
	}
	if _, err := attributesProblems(attrs, nil, reader, hashed); err == nil {
		t.Error("a listing of no tracked file passed")
	}
}
